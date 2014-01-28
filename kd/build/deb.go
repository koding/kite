package build

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"fmt"
	"go/build"
	"io"
	"os"
	"strings"
	"time"

	"github.com/blakesmith/ar"
)

const control = `Package: %s
Version: %s
Architecture: %s
Maintainer: Koding Developers <hello@koding.com>
Installed-Size: %d
Section: devel
Priority: extra
Description: %s Kite
`

func (b *Build) Linux() error {
	debName := b.Output + ".deb"
	tarFile := b.Output + ".tar.gz"

	deb, err := os.Create(debName + ".inprogress")
	if err != nil {
		return fmt.Errorf("cannot create deb: %v", err)
	}
	defer deb.Close()

	// create first tar file
	fmt.Println("creating tarfile", tarFile)
	if err := b.TarGzFile(); err != nil {
		return err
	}

	fmt.Println("openening tarfile", tarFile)
	tf, err := os.Open(tarFile)
	if err != nil {
		return err
	}
	defer tf.Close()

	fmt.Println("creating debfile from tarfile")
	if err := b.createDeb(tf, deb); err != nil {
		return err
	}

	if err := os.Rename(debName+".inprogress", debName); err != nil {
		return err
	}

	fmt.Println("package", debName, "ready")
	return nil
}

func debArch() string {
	arch := build.Default.GOARCH
	if arch == "386" {
		return "i386"
	}
	return arch
}

func (b *Build) createDeb(tarball io.Reader, deb io.Writer) error {
	now := time.Now()
	dataTarGz, md5sums, instSize, err := b.translateTarball(now, tarball)
	if err != nil {
		return err
	}

	controlTarGz, err := b.createControl(now, instSize, md5sums)
	if err != nil {
		return err
	}
	w := ar.NewWriter(deb)
	if err := w.WriteGlobalHeader(); err != nil {
		return fmt.Errorf("cannot write ar header to deb file: %v", err)
	}

	if err := addArFile(now, w, "debian-binary", []byte("2.0\n")); err != nil {
		return fmt.Errorf("cannot pack debian-binary: %v", err)
	}
	if err := addArFile(now, w, "control.tar.gz", controlTarGz); err != nil {
		return fmt.Errorf("cannot add control.tar.gz to deb: %v", err)
	}
	if err := addArFile(now, w, "data.tar.gz", dataTarGz); err != nil {
		return fmt.Errorf("cannot add data.tar.gz to deb: %v", err)
	}
	return nil
}

func addArFile(now time.Time, w *ar.Writer, name string, body []byte) error {
	hdr := ar.Header{
		Name:    name,
		Size:    int64(len(body)),
		Mode:    0644,
		ModTime: now,
	}
	if err := w.WriteHeader(&hdr); err != nil {
		return fmt.Errorf("cannot write file header: %v", err)
	}
	_, err := w.Write(body)
	return err
}

func (b *Build) translateTarball(now time.Time, tarball io.Reader) (dataTarGz, md5sums []byte, instSize int64, err error) {
	buf := &bytes.Buffer{}
	compress := gzip.NewWriter(buf)
	out := tar.NewWriter(compress)

	md5buf := &bytes.Buffer{}
	md5tmp := make([]byte, 0, md5.Size)

	uncompress, err := gzip.NewReader(tarball)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("cannot uncompress tarball: %v", err)
	}

	in := tar.NewReader(uncompress)
	first := true
	for {
		h, err := in.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, 0, fmt.Errorf("cannot read tarball: %v", err)
		}

		instSize += h.Size
		h.Name = strings.TrimLeft(h.Name, "./")
		if first && h.Name != b.AppName && h.Name != b.AppName+"/" {
			h := tar.Header{
				Name:     "./opt/koding/" + b.AppName,
				Mode:     0755,
				ModTime:  h.ModTime,
				Typeflag: tar.TypeDir,
			}

			if err := out.WriteHeader(&h); err != nil {
				return nil, nil, 0, fmt.Errorf("cannot write header of %s to data.tar.gz: %v", h.Name, err)
			}
		}

		const prefix = "./opt/koding/"
		h.Name = prefix + h.Name
		if h.Typeflag == tar.TypeDir && !strings.HasSuffix(h.Name, "/") {
			h.Name += "/"
		}

		if err := out.WriteHeader(h); err != nil {
			return nil, nil, 0, fmt.Errorf("cannot write header of %s to data.tar.gz: %v", h.Name, err)
		}

		fmt.Println("tar: packing", h.Name[len(prefix):])

		if h.Typeflag == tar.TypeDir {
			continue
		}

		digest := md5.New()
		if _, err := io.Copy(out, io.TeeReader(in, digest)); err != nil {
			return nil, nil, 0, err
		}

		fmt.Fprintf(md5buf, "%x  %s\n", digest.Sum(md5tmp), h.Name[2:])
	}

	if err := out.Close(); err != nil {
		return nil, nil, 0, err
	}

	if err := compress.Close(); err != nil {
		return nil, nil, 0, err
	}

	return buf.Bytes(), md5buf.Bytes(), instSize, nil
}

func addTarSymlink(now time.Time, out *tar.Writer, name, target string) error {
	h := tar.Header{
		Name:     name,
		Linkname: target,
		Mode:     0777,
		ModTime:  now,
		Typeflag: tar.TypeSymlink,
	}
	if err := out.WriteHeader(&h); err != nil {
		return fmt.Errorf("cannot write header of %s to data.tar.gz: %v", h.Name, err)
	}
	return nil
}

func (b *Build) createControl(now time.Time, instSize int64, md5sums []byte) (controlTarGz []byte, err error) {
	buf := &bytes.Buffer{}
	compress := gzip.NewWriter(buf)
	tarball := tar.NewWriter(compress)

	body := []byte(fmt.Sprintf(control, b.Version, debArch(), instSize/1024))
	hdr := tar.Header{
		Name:     "control",
		Size:     int64(len(body)),
		Mode:     0644,
		ModTime:  now,
		Typeflag: tar.TypeReg,
	}
	if err := tarball.WriteHeader(&hdr); err != nil {
		return nil, fmt.Errorf("cannot write header of control file to control.tar.gz: %v", err)
	}
	if _, err := tarball.Write(body); err != nil {
		return nil, fmt.Errorf("cannot write control file to control.tar.gz: %v", err)
	}

	hdr = tar.Header{
		Name:     "md5sums",
		Size:     int64(len(md5sums)),
		Mode:     0644,
		ModTime:  now,
		Typeflag: tar.TypeReg,
	}
	if err := tarball.WriteHeader(&hdr); err != nil {
		return nil, fmt.Errorf("cannot write header of md5sums file to control.tar.gz: %v", err)
	}
	if _, err := tarball.Write(md5sums); err != nil {
		return nil, fmt.Errorf("cannot write md5sums file to control.tar.gz: %v", err)
	}

	if err := tarball.Close(); err != nil {
		return nil, fmt.Errorf("closing control.tar.gz: %v", err)
	}
	if err := compress.Close(); err != nil {
		return nil, fmt.Errorf("closing control.tar.gz: %v", err)
	}
	return buf.Bytes(), nil
}
