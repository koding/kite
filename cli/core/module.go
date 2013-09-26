package core

type Module struct {
	SubModules map[string]*Module
	Command    *Command
	Definition string
}

func (m *Module) AddCommand(name string, command *Command) *Module {
	subModule := &Module{Command: command}
	m.SubModules[name] = subModule
	return subModule
}

func (m *Module) AddModule(name string, definition string) *Module {
	subModule := &Module{SubModules: make(map[string]*Module, 0), Definition: definition}
	m.SubModules[name] = subModule
	return subModule
}
