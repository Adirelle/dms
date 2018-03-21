package processor

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"strconv"
	"sync"
	"time"

	dms_cache "github.com/Adirelle/dms/pkg/cache"
	"github.com/Adirelle/dms/pkg/cds"
	"github.com/Adirelle/dms/pkg/didl_lite"
	"github.com/Adirelle/go-libs/cache"
	"github.com/Adirelle/go-libs/logging"
)

type FFProbeConfig struct {
	BinPath string
	Limit   uint
}

type FFProbeProcessor struct {
	binPath string
	l       logging.Logger
	c       cache.Cache
	lk      sync.Locker
}

func (FFProbeProcessor) String() string {
	return "FFProbeProcessor"
}

func NewFFProbeProcessor(c FFProbeConfig, cm *dms_cache.Manager, l logging.Logger) (p *FFProbeProcessor, err error) {
	realPath, err := exec.LookPath(c.BinPath)
	if err != nil {
		return
	}
	p = &FFProbeProcessor{binPath: realPath,
		l:  l,
		lk: concurrencyLock(make(chan struct{}, c.Limit)),
	}
	p.c = cm.Create("ffprobe", p.loader)
	return
}

func (p *FFProbeProcessor) Process(obj *cds.Object, ctx context.Context) {
	t := obj.MimeType.Type
	if !(t == "audio" || t == "video" || t == "image") {
		return
	}

	l := logging.MustFromContext(ctx)

	if err := p.probeObject(obj, ctx); err != nil {
		l.Error(err)
		return
	}

	for i := range obj.Resources {
		if err := p.probeResource(&obj.Resources[i], ctx); err != nil {
			l.Error(err)
			return
		}
	}
}

var tagMap = map[string]string{
	"artist": didl_lite.TagArtist,
	"album":  didl_lite.TagAlbum,
	"genre":  didl_lite.TagGenre,
}

func (p *FFProbeProcessor) probeObject(obj *cds.Object, ctx context.Context) error {
	info, err := p.probePath(obj.FilePath, ctx)
	if err != nil {
		return err
	}

	if title, ok := info.Format.Tags["title"]; ok {
		obj.Title = title
	}

	if createdStr, ok := info.Format.Tags["creation_time"]; ok {
		created, err := time.Parse(time.RFC3339Nano, createdStr)
		if err != nil {
			return err
		}
		obj.Date = created
	}

	obj.Artist = info.Format.Tags["artist"]
	obj.Genre = info.Format.Tags["genre"]
	obj.Album = info.Format.Tags["album"]

	return nil
}

func (p *FFProbeProcessor) probeResource(res *cds.Resource, ctx context.Context) error {
	info, err := p.probePath(res.FilePath, ctx)
	if err != nil {
		return err
	}

	hasVideo, hasAudio, hasDuration := false, false, false

	switch res.MimeType.Type {
	case "image":
		hasVideo = true
	case "video":
		hasVideo, hasAudio, hasDuration = true, true, true
	case "audio":
		hasAudio, hasDuration = true, true
	}

	if hasDuration {
		res.Bitrate = uint32(info.Format.BitRate.Int64())
		if info.Format.Duration != 0.0 {
			duration := float64(info.Format.Duration) * float64(time.Second)
			res.Duration = time.Duration(duration)
		}
	}

	gotVideo, gotAudio := false, false
	for _, s := range info.Streams {
		if hasVideo && s.CodecType == "video" && (!gotVideo || s.Disposition.Default == 1) {
			gotVideo = true
			res.Resolution.Width = s.Width
			res.Resolution.Height = s.Height
		}
		if hasAudio && s.CodecType == "audio" && (!gotAudio || s.Disposition.Default == 1) {
			gotAudio = true
			res.SampleFrequency = uint32(s.SampleRate.Int64())
			res.NrAudioChannels = uint8(s.Channels)
		}
	}

	return nil
}

func (p *FFProbeProcessor) probePath(path string, ctx context.Context) (info ffprobeInfo, err error) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		val, err := p.c.Get(path)
		if err == nil {
			info = val.(ffprobeInfo)
		}
	}()
	select {
	case <-done:
	case <-ctx.Done():
		err = ctx.Err()
	}
	return
}

func (p *FFProbeProcessor) loader(key interface{}) (value interface{}, err error) {
	p.lk.Lock()
	defer p.lk.Unlock()

	filePath := key.(string)
	l := p.l.With("path", filePath)

	cmd := exec.Command(p.binPath, "-i", filePath, "-of", "json", "-v", "error", "-show_format", "-show_streams")

	l.Debugf("running %v", cmd.Args)
	output, err := cmd.Output()
	if err != nil {
		return
	}

	info := ffprobeInfo{}
	err = json.NewDecoder(bytes.NewReader(output)).Decode(&info)
	if err == nil {
		value = info
	} else {
		l.Errorf("error unmarshalling: %s\n%s", err.Error(), output)
	}
	return
}

type ffprobeInfo struct {
	Streams []ffprobeStream `json:"streams"`
	Format  ffprobeFormat   `json:"format"`
}

type ffprobeStream struct {
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

type ffprobeFormat struct {
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

type concurrencyLock chan struct{}

func (c concurrencyLock) Lock() {
	var s struct{}
	c <- s
}

func (c concurrencyLock) Unlock() {
	select {
	case <-c:
	default:
	}
}
