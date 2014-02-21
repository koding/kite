package util

import (
	"io"
)

func JoinStreams(local, remote io.ReadWriteCloser) chan error {
	errc := make(chan error, 2)

	copy := func(dst io.WriteCloser, src io.ReadCloser) {
		_, err := io.Copy(dst, src)
		src.Close()
		dst.Close()
		errc <- err
	}

	go copy(local, remote)
	go copy(remote, local)

	return errc
}
