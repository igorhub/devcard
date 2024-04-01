package devcard

import (
	"fmt"
	"strings"

	"github.com/sanity-io/litter"
)

func valToString(val any) string {
	switch x := val.(type) {
	case string:
		return x
	case error:
		return x.Error()
	default:
		return fmt.Sprintf("%#v", x)
	}
}

func valsToString(vals []any) string {
	s := new(strings.Builder)
	for _, val := range vals {
		s.WriteString(valToString(val))
	}
	return s.String()
}

func pprint(val any) string {
	cfg := litter.Config
	cfg.HidePrivateFields = false
	return cfg.Sdump(val)
}
