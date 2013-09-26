package core

type Command struct {
	Help func() string
	Exec func() error
}
