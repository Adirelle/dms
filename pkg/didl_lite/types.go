package didl_lite

import (
	"fmt"
	"math"
	"time"
)

type Resolution struct {
	Width  uint
	Height uint
}

func (r *Resolution) String() string {
	return fmt.Sprintf("%dx%d", r.Width, r.Height)
}

func (r *Resolution) MarshalText() (text []byte, err error) {
	return []byte(r.String()), nil
}

type Duration time.Duration

func (d Duration) String() string {
	td := time.Duration(d)
	h := td / time.Hour
	m := (td / time.Minute) % 60
	s := math.Mod(float64(d)/float64(time.Second), 60.0)
	return fmt.Sprintf("%d:%02d:%02.6f", h, m, s)
}

func (d Duration) MarshalText() (text []byte, err error) {
	return []byte(d.String()), nil
}
