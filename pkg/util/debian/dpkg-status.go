package debian

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
)

// DpkgStatus reflects the content of a DPKG status file
type DpkgStatus struct {
	Index map[string]DpkgPackageStatus
}

// LoadDpkgStatus loads a dpkg status file from disk
func LoadDpkgStatus(fn string) (*DpkgStatus, error) {
	f, err := os.OpenFile(fn, os.O_RDONLY, 0600)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var (
		buf    []byte
		linenr int
	)
	idx := make(map[string]DpkgPackageStatus)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		linenr++

		if scanner.Text() == "" {
			if buf == nil {
				continue
			}

			stat := DpkgPackageStatus(buf)
			nme := stat.Name()
			if nme == "" {
				return nil, fmt.Errorf("error in line %d: package has no name", linenr-1)
			}

			idx[nme] = stat
			buf = nil

			continue
		}

		buf = append(buf, scanner.Bytes()...)
		buf = append(buf, '\n')
	}

	return &DpkgStatus{idx}, nil
}

// SaveDpkgStatus saves a dpkg status file
func SaveDpkgStatus(f io.Writer, stat *DpkgStatus) error {
	out := bufio.NewWriter(f)
	defer out.Flush()

	var keys []string
	for k := range stat.Index {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	for _, key := range keys {
		pkg := stat.Index[key]

		n, err := out.Write(pkg)
		if err != nil {
			return err
		}
		if n < len(pkg) {
			return io.ErrShortWrite
		}

		_, err = out.WriteRune('\n')
		if err != nil {
			return err
		}
	}

	out.Flush()
	return nil
}

// DpkgPackageStatus reflects the status of a single package
type DpkgPackageStatus []byte

var (
	nameField = []byte("Package:")
)

// Name returns the name of the package
func (s DpkgPackageStatus) Name() string {
	return s.getFieldValue(nameField)
}

func (s DpkgPackageStatus) getFieldValue(prefix []byte) string {
	rows := bytes.Split(s, []byte("\n"))

	var targetRow []byte
	for _, r := range rows {
		if !bytes.HasPrefix(r, prefix) {
			continue
		}

		targetRow = r
		break
	}
	if targetRow == nil {
		return ""
	}

	return string(bytes.TrimSpace(bytes.TrimPrefix(targetRow, prefix)))
}
