package util

import (
	"context"
	"fmt"
	"testing"
	"time"

	"database/sql"
	_ "github.com/herenow/go-crate"
)

type TestContainer interface {
	Container
	Start(t *testing.T)
	Stop(t *testing.T)
}

type DockerCrate struct {
	db *sql.DB
	c  DockerContainer
}

func (d *DockerCrate) IPAddress() string {
	return d.c.IPAddress()
}

func (d *DockerCrate) Start(t *testing.T) {
	d.c = NewDockerContainer("crate", 500*time.Millisecond)
	err := d.c.SpinUp(context.Background())
	if err != nil {
		t.Skip("Failed to start crate docker container", err)
	}

	d.db, err = sql.Open("crate",
		fmt.Sprintf("http://%s:4200/?timeout=%s",
			d.c.IPAddress(),
			(time.Second/time.Duration(PingTimeoutFrequency)).String()))
	if err != nil {
		t.Fatal(err)
	}

	err = PingTimeout(d.db, 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
}

func (d *DockerCrate) Stop(t *testing.T) {
	err := d.db.Close()
	if err != nil {
		t.Fatal(err)
	}
	err = d.c.SpinDown(context.Background())
	if err != nil {
		t.Fatal(err)
	}
}

func StartDockerCrate(t *testing.T) *DockerCrate {
	d := DockerCrate{}
	d.Start(t)
	return &d
}
