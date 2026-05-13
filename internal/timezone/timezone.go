package timezone

import (
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"
)

const Default = "Asia/Shanghai"

func Normalize(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = Default
	}
	if _, err := time.LoadLocation(name); err != nil {
		return "", fmt.Errorf("invalid time zone %q", name)
	}
	return name, nil
}

func Load(name string) *time.Location {
	name, err := Normalize(name)
	if err != nil {
		name = Default
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.FixedZone("CST", 8*60*60)
	}
	return loc
}
