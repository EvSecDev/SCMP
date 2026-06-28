package drn

import (
	"strings"
)

// Converts a namespace slice and field slice to raw (unvalidated) DRN string
func QuickFormat(namespace []string, fields ...string) (drnStr string) {
	ns := strings.Join(namespace, NamespaceSeparator)
	df := strings.Join(fields, FieldSeparator)
	drnStr = OpenDelimiter + Prefix + ns + PrimarySeparator + df + CloseDelimiter
	return
}
