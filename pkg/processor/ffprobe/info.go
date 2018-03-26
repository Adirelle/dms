package ffprobe

import (
	"encoding/gob"
	"strconv"

	"github.com/Adirelle/dms/pkg/filesystem"
)

func init() {
	gob.RegisterName("ffprobe.Info", Info{})
}

type Info struct {
	filesystem.FileItem
	Streams []Stream `json:"streams"`
	Format  Format   `json:"format"`
}

type Stream struct {
	CodecName   string        `json:"codec_name"`
	CodecType   string        `json:"codec_type"`
	Width       uint          `json:"width"`
	Height      uint          `json:"height"`
	SampleRate  integerString `json:"sample_rate"`
	Channels    uint          `json:"channels"`
	Disposition struct {
		Default int `json:"default"`
	} `json:"disposition"`
}

type Format struct {
	Duration floatString       `json:"duration"`
	Size     integerString     `json:"size"`
	BitRate  integerString     `json:"bit_rate"`
	Tags     map[string]string `json:"tags"`
}

type floatString float64

func (s floatString) Float64() float64 {
	return float64(s)
}

func (s *floatString) UnmarshalText(t []byte) (err error) {
	f, err := strconv.ParseFloat(string(t), 64)
	if err == nil {
		(*s) = floatString(f)
	}
	return
}

type integerString int64

func (s integerString) Int64() int64 {
	return int64(s)
}

func (s *integerString) UnmarshalText(t []byte) (err error) {
	i, err := strconv.ParseInt(string(t), 10, 64)
	if err == nil {
		(*s) = integerString(i)
	}
	return
}
