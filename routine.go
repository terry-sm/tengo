package tengo

import (
	"fmt"
	"strings"
)

// RoutineType distinguishes between stored procedures and functions
type RoutineType string

// Constants enumerating valid routine types
const (
	RoutineTypeProc RoutineType = "procedure"
	RoutineTypeFunc RoutineType = "function"
)

// Routine represents a stored procedure or function.
type Routine struct {
	Name              string
	Type              RoutineType
	Body              string
	ParamString       string // Formatted as per original CREATE
	ReturnDataType    string // Includes charset/collation when relevant
	Definer           string
	DatabaseCollation string // from creation time
	Comment           string
	Deterministic     bool
	SQLDataAccess     string
	SecurityType      string
	SQLMode           string // sql_mode in effect at creation time
}

// Definition generates and returns a CREATE PROCEDURE or CREATE FUNCTION
// statement based on the Routine's Go field values.
func (r *Routine) Definition(_ Flavor) string {
	var definer, returnClause, characteristics string

	atPos := strings.LastIndex(r.Definer, "@")
	if atPos >= 0 {
		definer = fmt.Sprintf("%s@%s", EscapeIdentifier(r.Definer[0:atPos]), EscapeIdentifier(r.Definer[atPos+1:]))
	}
	if r.Type == RoutineTypeFunc {
		returnClause = fmt.Sprintf(" RETURNS %s", r.ReturnDataType)
	}

	clauses := make([]string, 0)
	if r.Comment != "" {
		clauses = append(clauses, fmt.Sprintf("    COMMENT '%s'\n", EscapeValueForCreateTable(r.Comment)))
	}
	if r.Deterministic {
		clauses = append(clauses, "    DETERMINISTIC\n")
	}
	if r.SQLDataAccess != "CONTAINS SQL" {
		clauses = append(clauses, fmt.Sprintf("    %s\n", r.SQLDataAccess))
	}
	if r.SecurityType != "DEFINER" {
		clauses = append(clauses, fmt.Sprintf("    SQL SECURITY %s\n", r.SecurityType))
	}
	characteristics = strings.Join(clauses, "")

	return fmt.Sprintf("CREATE %s DEFINER=%s %s(%s)%s\n%s%s",
		strings.ToUpper(string(r.Type)),
		definer,
		EscapeIdentifier(r.Name),
		r.ParamString,
		returnClause,
		characteristics,
		r.Body)
}

// Equals returns true if two routines are identical, false otherwise.
func (r *Routine) Equals(other *Routine) bool {
	// shortcut if both nil pointers, or both pointing to same underlying struct
	if r == other {
		return true
	}
	// if one is nil, but the two pointers aren't equal, then one is non-nil
	if r == nil || other == nil {
		return false
	}

	// All fields are simple scalars, so we can just use equality check once we
	// know neither is nil
	return *r == *other
}
