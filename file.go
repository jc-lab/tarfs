package tarfs

import (
	"archive/tar"
	"errors"
	"io"
	"io/fs"
	"path"
	"strings"
)

var _ fs.File = (*file)(nil)

// File implements fs.File.
type file struct {
	h  *tar.Header
	sr *io.SectionReader
	r  *tar.Reader
	p  int64
}

func (f *file) Close() error {
	return nil
}

func (f *file) Read(b []byte) (int, error) {
	n, err := f.r.Read(b)
	if n > 0 {
		f.p += int64(n)
	}
	return n, err
}

func (f *file) Stat() (fs.FileInfo, error) {
	return f.h.FileInfo(), nil
}

func (f *file) reopen() error {
	if _, err := f.sr.Seek(0, io.SeekStart); err != nil {
		return err
	}
	r := tar.NewReader(f.sr)
	if _, err := r.Next(); err != nil {
		return err
	}
	f.p = 0
	f.r = r
	return nil
}

func (f *file) Seek(offset int64, whence int) (int64, error) {
	var newp int64
	switch whence {
	case io.SeekStart:
		newp = offset
	case io.SeekCurrent:
		if offset >= 0 {
			if err := discard(f, offset); err != nil {
				return 0, err
			}
			return f.p, nil
		}
		newp = f.p + offset
	case io.SeekEnd:
		stat, err := f.Stat()
		if err != nil {
			return 0, err
		}
		newp = stat.Size() + offset
	default:
		return 0, errors.New("invalid whence")
	}
	if newp < 0 {
		return 0, errors.New("negative position")
	}
	if err := f.reopen(); err != nil {
		return 0, err
	}
	if err := discard(f, newp); err != nil {
		return 0, err
	}
	return f.p, nil
}

func discard(r io.Reader, size int64) error {
	buf := make([]byte, 4096)
	var totalRead int64

	for totalRead < size {
		bytesToRead := size - totalRead
		if bytesToRead > int64(len(buf)) {
			bytesToRead = int64(len(buf))
		}

		n, err := r.Read(buf[:bytesToRead])
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		totalRead += int64(n)
	}

	if totalRead < size {
		return errors.New("failed to discard the specified amount of data")
	}

	return nil
}

var _ fs.ReadDirFile = (*dir)(nil)

// Dir implements fs.ReadDirFile.
type dir struct {
	h   *tar.Header
	es  []fs.DirEntry
	pos int
}

func (*dir) Close() error                 { return nil }
func (*dir) Read(_ []byte) (int, error)   { return 0, io.EOF }
func (d *dir) Stat() (fs.FileInfo, error) { return d.h.FileInfo(), nil }
func (d *dir) ReadDir(n int) ([]fs.DirEntry, error) {
	es := d.es[d.pos:]
	if len(es) == 0 {
		if n == -1 {
			return nil, nil
		}
		return nil, io.EOF
	}
	end := min(len(es), n)
	if n == -1 {
		end = len(es)
	}
	d.pos += end
	return es[:end], nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type dirent struct{ *tar.Header }

var _ fs.DirEntry = dirent{}

func (d dirent) Name() string               { return path.Base(d.Header.Name) }
func (d dirent) IsDir() bool                { return d.Header.FileInfo().IsDir() }
func (d dirent) Type() fs.FileMode          { return d.Header.FileInfo().Mode() & fs.ModeType }
func (d dirent) Info() (fs.FileInfo, error) { return d.FileInfo(), nil }

// SortDirent returns a function suitable to pass to sort.Slice as a "cmp"
// function.
//
// This is needed because the fs interfaces specify that DirEntry slices
// returned by the ReadDir methods are sorted lexically.
func sortDirent(s []fs.DirEntry) func(i, j int) bool {
	return func(i, j int) bool {
		return strings.Compare(s[i].Name(), s[j].Name()) == -1
	}
}
