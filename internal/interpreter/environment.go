package interpreter

// Environment stores variables with optional parent scope chaining.
type Environment struct {
	store  map[string]*Result
	parent *Environment
}

// NewEnvironment creates a new environment with no parent.
func NewEnvironment() *Environment {
	return &Environment{store: make(map[string]*Result)}
}

// NewEnclosed creates a child environment that falls through to parent on lookup.
func NewEnclosed(parent *Environment) *Environment {
	return &Environment{
		store:  make(map[string]*Result),
		parent: parent,
	}
}

// Get retrieves a variable by name. Walks up the parent chain.
func (e *Environment) Get(name string) (*Result, bool) {
	val, ok := e.store[name]
	if !ok && e.parent != nil {
		return e.parent.Get(name)
	}
	return val, ok
}

// Set stores a variable in the current scope.
func (e *Environment) Set(name string, val *Result) {
	e.store[name] = val
}
