// Package util contains the shared functions and constants for cli package.
package util

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// got it from http://golang.org/misc/dist/bindist.go?m=text and removed go
// related stuff, works perfect. It creates a tar.gz container from the given
// workdir.
func MakeTar(targ, workdir string) error {
	f, err := os.Create(targ)
	if err != nil {
		return err
	}
	zout := gzip.NewWriter(f)
	tw := tar.NewWriter(zout)

	err = filepath.Walk(workdir, func(path string, fi os.FileInfo, err error) error {
		if !strings.HasPrefix(path, workdir) {
			log.Panicf("walked filename %q doesn't begin with workdir %q", path, workdir)
		}
		name := path[len(workdir):]

		// Chop of any leading / from filename, leftover from removing workdir.
		if strings.HasPrefix(name, "/") {
			name = name[1:]
		}

		if name == "" {
			return nil // do not inclue empty paths
		}

		// log.Printf("adding to tar: %s", name)

		target, _ := os.Readlink(path)
		hdr, err := tar.FileInfoHeader(fi, target)
		if err != nil {
			return err
		}

		hdr.Name = name
		hdr.Uname = "root"
		hdr.Gname = "root"
		hdr.Uid = 0
		hdr.Gid = 0

		// Force permissions to 0755 for executables, 0644 for everything else.
		if fi.Mode().Perm()&0111 != 0 {
			hdr.Mode = hdr.Mode&^0777 | 0755
		} else {
			hdr.Mode = hdr.Mode&^0777 | 0644
		}

		err = tw.WriteHeader(hdr)
		if err != nil {
			return fmt.Errorf("Error writing file %q: %v", name, err)
		}

		if fi.IsDir() {
			return nil
		}

		r, err := os.Open(path)
		if err != nil {
			return err
		}
		defer r.Close()

		_, err = io.Copy(tw, r)
		return err
	})

	if err != nil {
		return err
	}

	if err := tw.Close(); err != nil {
		return err
	}

	if err := zout.Close(); err != nil {
		return err
	}

	return f.Close()
}

// Copy copies the file or directory from source path to destination path.
// Directories are copied recursively. Copy does not handle symlinks currently.
func Copy(src, dst string) error {
	if dst == "." {
		dst = filepath.Base(src)
	}

	if src == dst {
		return fmt.Errorf("%s and %s are identical (not copied).", src, dst)
	}

	if !Exists(src) {
		return fmt.Errorf("%s: no such file or directory.", src)
	}

	if Exists(dst) && IsFile(dst) {
		return fmt.Errorf("%s is a directory (not copied).", src)
	}

	srcBase, _ := filepath.Split(src)
	walks := 0

	// dstPath returns the rewritten destination path for the given source path
	dstPath := func(srcPath string) string {
		srcPath = strings.TrimPrefix(srcPath, srcBase)

		// foo/example/hello.txt -> bar/example/hello.txt
		if walks != 0 {
			return filepath.Join(dst, srcPath)
		}

		// hello.txt -> example/hello.txt
		if Exists(dst) && !IsFile(dst) {
			return filepath.Join(dst, filepath.Base(srcPath))
		}

		// hello.txt -> test.txt
		return dst
	}

	filepath.Walk(src, func(srcPath string, file os.FileInfo, err error) error {
		defer func() { walks++ }()

		if file.IsDir() {
			os.MkdirAll(dstPath(srcPath), 0755)
		} else {
			err = copyFile(srcPath, dstPath(srcPath))
			if err != nil {
				fmt.Println(err)
			}
		}

		return nil
	})

	return nil
}

// IsFile checks wether the given file is a directory or not. It panics if an
// error is occured. Use IsFileOk to use the returned error.
func IsFile(file string) bool {
	ok, err := IsFileOk(file)
	if err != nil {
		panic(err)
	}

	return ok
}

// IsFileOk checks whether the given file is a directory or not.
func IsFileOk(file string) (bool, error) {
	sf, err := os.Open(file)
	if err != nil {
		return false, err
	}
	defer sf.Close()

	fi, err := sf.Stat()
	if err != nil {
		return false, err
	}

	if fi.IsDir() {
		return false, nil
	}

	return true, nil
}

// Exists checks whether the given file exists or not. It panics if an error
// is occured. Use ExistsOk to use the returned error.
func Exists(file string) bool {
	ok, err := ExistsOk(file)
	if err != nil {
		panic(err)
	}

	return ok
}

// ExistsOk checks whether the given file exists or not.
func ExistsOk(file string) (bool, error) {
	_, err := os.Stat(file)
	if err == nil {
		return true, nil // file exist
	}

	if os.IsNotExist(err) {
		return false, nil // file does not exist
	}

	return false, err
}

// Use Copy() instead of copyFile(), it handles directories and recursive
// copying too.
func copyFile(src, dst string) error {
	sf, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sf.Close()

	fi, err := sf.Stat()
	if err != nil {
		return err
	}

	if fi.IsDir() {
		return errors.New("src is a directory, please provide a file")
	}

	df, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode())
	if err != nil {
		return err
	}
	defer df.Close()

	if _, err := io.Copy(df, sf); err != nil {
		return err
	}

	return nil
}
