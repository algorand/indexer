package internal

import "fmt"

// StructField is used to pass data from the inner reflection analysis to the code generation.
type StructField struct {
	TagPath    string
	FieldPath  string
	CastPrefix string
	CastPost   string
}

// ReturnValue builds a return value given a variable to wrap parts around.
func ReturnValue(sf StructField, varName string) string {
	return fmt.Sprintf("%s%s.%s%s", sf.CastPrefix, varName, sf.FieldPath, sf.CastPost)
}
