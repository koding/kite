package core

type ModuleNode struct {
	SubModules       map[string]*ModuleNode
	Command          *Command
	ModuleDefinition string
}

func (m *ModuleNode) AddCommand(name string, command *Command) *ModuleNode {
	subModuleNode := &ModuleNode{Command: command}
	m.SubModules[name] = subModuleNode
	return subModuleNode
}

func (m *ModuleNode) AddModule(name string, definition string) *ModuleNode {
	subModuleNode := &ModuleNode{SubModules: make(map[string]*ModuleNode, 0), ModuleDefinition: definition}
	m.SubModules[name] = subModuleNode
	return subModuleNode
}
