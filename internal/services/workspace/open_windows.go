package workspace

import "os"

func openReadOnly(root *os.Root, path string) (*os.File, error) {
	return root.Open(path)
}
