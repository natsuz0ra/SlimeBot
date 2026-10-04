package agent

import (
	"fmt"
	"os"
)

// Owner prevents another process from recovering or claiming this database's
// live turns. The OS releases the lock if its process crashes.
type Owner struct{ file *os.File }

func AcquireOwner(path string) (*Owner, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = lockOwner(file); err != nil {
		file.Close()
		return nil, fmt.Errorf("another SlimeBot process owns this database: %w", err)
	}
	return &Owner{file}, nil
}
func (o *Owner) Close() error {
	if o == nil || o.file == nil {
		return nil
	}
	return o.file.Close()
}
