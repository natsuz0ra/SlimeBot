package privacy

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	mappingDirName   = "mappings-v1"
	mappingKeyFile   = "key-check"
	mappingFileExt   = ".map"
	mappingTemp      = ".tmp-"
	mappingHeaderLen = 8 + 28 // encrypted activity time: 8-byte UnixNano, 12-byte nonce, 16-byte tag
	maxMappingFile   = mappingHeaderLen + 28 + 2 + maxMappingBytes + maxMappingEntries*(1+4+maxPlaceholderBytes)
)

var (
	errMappingKeyMissing  = errors.New("set SLIMEBOT_PRIVACY_MAPPING_KEY to enable encrypted privacy mappings")
	errMappingKeyInvalid  = errors.New("SLIMEBOT_PRIVACY_MAPPING_KEY must be a base64-encoded 32-byte key")
	errMappingKeyMismatch = errors.New("the deployment key does not match the encrypted privacy mapping store")
	errMappingStorage     = errors.New("encrypted privacy mapping storage is unavailable")
	storedPlaceholderRE   = regexp.MustCompile(`^\{\{SB_[A-Z][A-Z0-9_]{0,23}_[0-9a-f]{32}\}\}$`)
)

type mappingDisk struct {
	dir    string
	aead   cipher.AEAD
	keyErr error
	ready  bool
}

func newMappingDisk(dir string) mappingDisk {
	d := mappingDisk{dir: filepath.Join(dir, mappingDirName)}
	encoded := os.Getenv("SLIMEBOT_PRIVACY_MAPPING_KEY")
	if encoded == "" {
		d.keyErr = errMappingKeyMissing
		return d
	}
	var decoder *base64.Encoding
	switch len(encoded) {
	case base64.StdEncoding.EncodedLen(32):
		decoder = base64.StdEncoding.Strict()
	case base64.RawStdEncoding.EncodedLen(32):
		decoder = base64.RawStdEncoding.Strict()
	default:
		d.keyErr = errMappingKeyInvalid
		return d
	}
	var key [32]byte
	n, err := decoder.Decode(key[:], []byte(encoded))
	if err != nil || n != len(key) || strings.ContainsAny(encoded, "\r\n") {
		clear(key[:])
		d.keyErr = errMappingKeyInvalid
		return d
	}
	block, err := aes.NewCipher(key[:])
	clear(key[:])
	if err == nil {
		d.aead, err = cipher.NewGCMWithRandomNonce(block)
	}
	if err != nil {
		d.keyErr = errMappingKeyInvalid
	}
	return d
}

// The opaque but key-independent name lets explicit session deletion work
// even during a deployment key outage. The name is authenticated as AAD.
func mappingName(sessionID string) string {
	h := sha256.Sum256([]byte("slimebot/privacy/mapping/v1\x00" + sessionID))
	return hex.EncodeToString(h[:]) + mappingFileExt
}

func isMappingName(name string) bool {
	if len(name) != 64+len(mappingFileExt) || !strings.HasSuffix(name, mappingFileExt) {
		return false
	}
	var buf [32]byte
	_, err := hex.Decode(buf[:], []byte(name[:64]))
	return err == nil && name == strings.ToLower(name)
}

func (d *mappingDisk) directoryExists() (bool, error) {
	st, err := os.Lstat(d.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil || !st.IsDir() {
		return false, errMappingStorage
	}
	return true, nil
}

func (d *mappingDisk) list() ([]os.DirEntry, error) {
	f, err := os.Open(d.dir)
	if err != nil {
		return nil, errMappingStorage
	}
	defer f.Close()
	const limit = 2*maxMappingScopes + 4
	entries, err := f.ReadDir(limit)
	if (err != nil && !errors.Is(err, io.EOF)) || len(entries) >= limit {
		return nil, errMappingStorage
	}
	return entries, nil
}

func (d *mappingDisk) ensureReady() error {
	if d.keyErr != nil {
		return d.keyErr
	}
	if d.ready {
		return nil
	}
	exists, err := d.directoryExists()
	if err != nil {
		return err
	}
	if !exists && os.MkdirAll(d.dir, 0o700) != nil {
		return errMappingStorage
	}
	const marker = "slimebot/privacy/mapping/key-check/v1"
	path := filepath.Join(d.dir, mappingKeyFile)
	f, _, err := openMappingFile(path, len(marker)+d.aead.Overhead())
	if errors.Is(err, fs.ErrNotExist) {
		entries, err := d.list()
		if err != nil {
			return err
		}
		for _, file := range entries {
			if isMappingName(file.Name()) {
				// A missing key check must not authorize replacement of existing data.
				return errMappingStorage
			}
		}
		if err = d.write(mappingKeyFile, d.aead.Seal(nil, nil, []byte(marker), []byte(marker))); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		data, readErr := io.ReadAll(io.LimitReader(f, int64(len(marker)+d.aead.Overhead()+1)))
		f.Close()
		if readErr != nil {
			return errMappingStorage
		}
		plain, err := d.aead.Open(nil, nil, data, []byte(marker))
		if err != nil || !bytes.Equal(plain, []byte(marker)) {
			return errMappingKeyMismatch
		}
	}
	d.ready = true
	return nil
}

func openMappingFile(path string, maxSize int) (*os.File, int64, error) {
	st, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0, fs.ErrNotExist
	}
	if err != nil || !st.Mode().IsRegular() || st.Size() > int64(maxSize) {
		return nil, 0, errMappingStorage
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, errMappingStorage
	}
	return f, st.Size(), nil
}

func (d *mappingDisk) fileExists(name string) (bool, error) {
	exists, err := d.directoryExists()
	if err != nil || !exists {
		return false, err
	}
	st, err := os.Lstat(filepath.Join(d.dir, name))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil || !st.Mode().IsRegular() || st.Size() > int64(maxMappingFile) {
		return false, errMappingStorage
	}
	return true, nil
}

func (d *mappingDisk) delete(name string) error {
	exists, err := d.directoryExists()
	if err != nil || !exists {
		return err
	}
	if err := os.Remove(filepath.Join(d.dir, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return errMappingStorage
	}
	return nil
}

func (d *mappingDisk) purge() error {
	d.ready = false
	// RemoveAll does not follow symlinks; the versioned directory contains only mappings.
	if err := os.RemoveAll(d.dir); err != nil {
		return errMappingStorage
	}
	return nil
}

func (d *mappingDisk) write(name string, data []byte) error {
	f, err := os.CreateTemp(d.dir, mappingTemp)
	if err != nil {
		return errMappingStorage
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil || os.Rename(f.Name(), filepath.Join(d.dir, name)) != nil {
		return errMappingStorage
	}
	return nil
}

func (d *mappingDisk) readHeader(r io.Reader, name string) ([]byte, time.Time, error) {
	header := make([]byte, mappingHeaderLen)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, time.Time{}, errMappingStorage
	}
	plain, err := d.aead.Open(nil, nil, header, []byte("time:"+name))
	if err != nil || len(plain) != 8 {
		return nil, time.Time{}, errMappingStorage
	}
	return header, time.Unix(0, int64(binary.BigEndian.Uint64(plain))), nil
}

// Cleanup authenticates only the small timestamp headers; values are decrypted
// only when their session actually needs them.
func (d *mappingDisk) sweep(now time.Time, ttl time.Duration, room bool) error {
	if err := d.ensureReady(); err != nil {
		return err
	}
	files, err := d.list()
	if err != nil {
		return err
	}
	var survivors []diskAge
	for _, file := range files {
		name := file.Name()
		if strings.HasPrefix(name, mappingTemp) {
			// A crash can leave a temporary file containing ciphertext.
			err = errors.Join(err, d.delete(name))
			continue
		}
		if !isMappingName(name) {
			continue
		}
		f, size, openErr := openMappingFile(filepath.Join(d.dir, name), maxMappingFile)
		if openErr != nil {
			err = errMappingStorage
			continue
		}
		_, used, headerErr := d.readHeader(f, name)
		f.Close()
		if headerErr != nil || size < mappingHeaderLen+int64(d.aead.Overhead())+2 {
			// Do not delete unverifiable data (e.g. a storage or key problem).
			err = errMappingStorage
			continue
		}
		if mappingExpired(used, now, ttl) {
			err = errors.Join(err, d.delete(name))
		} else {
			survivors = append(survivors, diskAge{name, used})
		}
	}
	if err != nil {
		return errMappingStorage
	}
	limit := maxMappingScopes
	if room {
		limit--
	}
	for len(survivors) > limit {
		old := 0
		for i := range survivors {
			if survivors[i].used.Before(survivors[old].used) {
				old = i
			}
		}
		if err = d.delete(survivors[old].name); err != nil {
			return err
		}
		survivors[old] = survivors[len(survivors)-1]
		survivors = survivors[:len(survivors)-1]
	}
	return nil
}

type diskAge struct {
	name string
	used time.Time
}

func (d *mappingDisk) load(name string, now time.Time, ttl time.Duration) (scopeState, error) {
	var state scopeState
	exists, err := d.fileExists(name)
	if err != nil || !exists {
		return state, err
	}
	if err = d.ensureReady(); err != nil {
		return state, err
	}
	f, _, err := openMappingFile(filepath.Join(d.dir, name), maxMappingFile)
	if err != nil {
		return state, errMappingStorage
	}
	header, used, err := d.readHeader(f, name)
	if err != nil || mappingExpired(used, now, ttl) {
		f.Close()
		if err != nil {
			return state, err
		}
		return state, d.delete(name)
	}
	data, err := io.ReadAll(io.LimitReader(f, int64(maxMappingFile-mappingHeaderLen+1)))
	f.Close()
	if err != nil || len(data) > maxMappingFile-mappingHeaderLen {
		return state, errMappingStorage
	}
	plain, err := d.aead.Open(data[:0], nil, data, append([]byte("values:"+name), header...))
	if err != nil {
		return state, errMappingStorage
	}
	defer clear(plain)
	values, err := decodeMapping(plain)
	return scopeState{values: values, used: used}, err
}

func (d *mappingDisk) save(name string, state scopeState, now time.Time, ttl time.Duration) error {
	if err := d.ensureReady(); err != nil {
		return err
	}
	exists, err := d.fileExists(name)
	if err != nil {
		return err
	}
	const timeBytes = 8
	if !exists {
		if err = d.sweep(now, ttl, true); err != nil {
			return err
		}
	} else {
		// Even cached sessions must not overwrite a changed or unreadable file.
		if _, err := d.load(name, now, ttl); err != nil {
			return err
		}
	}
	header := d.aead.Seal(nil, nil, binary.BigEndian.AppendUint64(make([]byte, 0, timeBytes), uint64(state.used.UnixNano())), []byte("time:"+name))
	plain := encodeMapping(state.values)
	defer clear(plain)
	return d.write(name, d.aead.Seal(header, nil, plain, append([]byte("values:"+name), header...)))
}

func encodeMapping(values map[string]entry) []byte {
	data := binary.BigEndian.AppendUint16(nil, uint16(len(values)))
	for p, v := range values {
		data = append(data, byte(len(p)))
		data = append(data, p...)
		data = binary.BigEndian.AppendUint32(data, uint32(len(v.Value)))
		data = append(data, v.Value...)
	}
	return data
}

func decodeMapping(data []byte) (map[string]entry, error) {
	if len(data) < 2 {
		return nil, errMappingStorage
	}
	count := int(binary.BigEndian.Uint16(data))
	data = data[2:]
	if count < 1 || count > maxMappingEntries || count*minPlaceholderBytes > len(data) {
		return nil, errMappingStorage
	}
	values := make(map[string]entry, count)
	valueBytes := 0
	for range count {
		if len(data) < 1 {
			return nil, errMappingStorage
		}
		pLen := int(data[0])
		data = data[1:]
		if pLen < minPlaceholderBytes || pLen > maxPlaceholderBytes || len(data) < pLen+4 {
			return nil, errMappingStorage
		}
		p := string(data[:pLen])
		if !storedPlaceholderRE.MatchString(p) {
			return nil, errMappingStorage
		}
		// The canonical placeholder stores its uppercase kind before the 32 hex digits.
		kind := p[len("{{SB_") : pLen-len("_0123456789abcdef0123456789abcdef}}")]

		n := binary.BigEndian.Uint32(data[pLen:])
		data = data[pLen+4:]
		if n < 1 || n > maxMappingValue || int(n) > len(data) || valueBytes+int(n) > maxMappingBytes {
			return nil, errMappingStorage
		}
		if _, duplicate := values[p]; duplicate {
			return nil, errMappingStorage
		}
		values[p] = entry{Value: string(data[:n]), Kind: kind}
		valueBytes += int(n)
		data = data[n:]
	}
	if len(data) != 0 {
		return nil, errMappingStorage
	}
	return values, nil
}
