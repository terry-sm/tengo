// Package tengo (Go La Tengo) is a database automation library. In its current
// form, its functionality is focused on MySQL schema introspection and
// diff'ing. Future releases will add more general-purpose automation features.
package tengo

import (
	"strings"
)

// ObjectType defines a class of object in a relational database system.
type ObjectType string

// Constants enumerating valid object types.
// Currently we do not define separate types for sub-types such as columns,
// indexes, foreign keys, etc as these are handled within the table logic.
const (
	ObjectTypeDatabase ObjectType = "database"
	ObjectTypeTable    ObjectType = "table"
	ObjectTypeProc     ObjectType = "procedure"
	ObjectTypeFunc     ObjectType = "function"
)

// Caps returns the object type as an uppercase string.
func (ot ObjectType) Caps() string {
	return strings.ToUpper(string(ot))
}
