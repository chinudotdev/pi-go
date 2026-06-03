// Package sourceinfo tracks where a resource was loaded from.
package sourceinfo

// Scope indicates the scope of a source.
type Scope string

const (
	ScopeUser      Scope = "user"
	ScopeProject   Scope = "project"
	ScopeTemporary Scope = "temporary"
)

// Origin indicates whether the source is a package or top-level.
type Origin string

const (
	OriginPackage  Origin = "package"
	OriginTopLevel Origin = "top-level"
)

// Info describes where a resource file came from.
type Info struct {
	Path    string
	Source  string
	Scope   Scope
	Origin  Origin
	BaseDir string
}

// Synthetic creates a SourceInfo for a resource that wasn't loaded from a
// package manager (e.g., discovered on the local filesystem).
func Synthetic(path string, source string, scope ...Scope) Info {
	s := ScopeTemporary
	if len(scope) > 0 {
		s = scope[0]
	}
	return Info{
		Path:   path,
		Source: source,
		Scope:  s,
		Origin: OriginTopLevel,
	}
}
