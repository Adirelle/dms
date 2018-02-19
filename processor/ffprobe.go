package processor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/anacrolix/dms/cds"
	"github.com/anacrolix/dms/logging"
	"github.com/bluele/gcache"
)

type FFProbeProcessor struct {
	binPath string
	l       logging.Logger
	cache   ffprobeCache
}

func NewFFProbeProcessor(path string, logger logging.Logger) (p *FFProbeProcessor, err error) {
	realPath, err := exec.LookPath(path)
	if err == nil {
		p = &FFProbeProcessor{binPath: realPath, l: logger}
		p.cache = ffprobeCache{gcache.New(1000).ARC().LoaderFunc(p.doProbe).Build()}
	}
	return
}

func (p *FFProbeProcessor) Process(obj *cds.Object) {
	mType := obj.MimeType().Type
	if !(mType == "audio" || mType == "video" || mType == "image") {
		return
	}

	group := sync.WaitGroup{}
	group.Add(1 + len(obj.Res))

	go func() {
		defer group.Done()
		p.probeObject(obj)
	}()

	for i := range obj.Res {
		go func(res *cds.Resource) {
			defer group.Done()
			p.probeResource(mType, res)
		}(&obj.Res[i])
	}

	group.Wait()
}

var tagMap = map[string]string{
	"artist": "upnp:artist",
	"album":  "upnp:album",
	"genre":  "upnp:genre",
}

func (p *FFProbeProcessor) probeObject(obj *cds.Object) {
	info, err := p.cache.Get(obj.FilePath)
	if err != nil {
		return
	}

	if title, ok := info.Format.Tags["title"]; ok {
		obj.Title = title
	}

	if createdStr, ok := info.Format.Tags["creation_time"]; ok {
		created, err := time.Parse(time.RFC3339Nano, createdStr)
		if err == nil {
			obj.Tags.Set("upnp:date", created.Format(time.RFC3339))
		}
	}

	for tagName, attrName := range tagMap {
		if value, ok := info.Format.Tags[tagName]; ok {
			obj.Tags.Set(attrName, value)
		}
	}
}

func (p *FFProbeProcessor) probeResource(mainType string, res *cds.Resource) {
	info, err := p.cache.Get(res.FilePath)
	if err != nil {
		return
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
		res.SetTag("bitrate", info.Format.BitRate)
		if info.Format.Duration != 0.0 {
			res.SetTag("duration", formatDuration(float64(info.Format.Duration)))
		}
	}

	gotVideo, gotAudio := false, false
	for _, s := range info.Streams {
		if hasVideo && s.CodecType == "video" && (!gotVideo || s.Disposition.Default == 1) {
			gotVideo = true
			if s.Width != 0 && s.Height != 0 {
				res.SetTag("resolution", fmt.Sprintf("%dx%d", s.Width, s.Height))
			}
		}
		if hasAudio && s.CodecType == "audio" && (!gotAudio || s.Disposition.Default == 1) {
			gotAudio = true
			if s.SampleRate != "" {
				res.SetTag("sampleFrequency", s.SampleRate)
			}
			if s.Channels != 0 {
				res.SetTag("nrAudioChannels", strconv.Itoa(int(s.Channels)))
			}
		}
	}
}

type ffprobeCache struct{ gcache.Cache }

func (c ffprobeCache) Get(path string) (ffprobeInfo, error) {
	v, err := c.Cache.Get(path)
	return v.(ffprobeInfo), err
}

func (p *FFProbeProcessor) doProbe(path interface{}) (data interface{}, err error) {
	cmd := exec.Command(p.binPath, "-i", path.(string), "-of", "json", "-v", "error", "-show_format", "-show_streams")

	p.l.Debugf("Running %v", cmd.Args)
	output, err := cmd.Output()
	if err != nil {
		return
	}

	info := ffprobeInfo{}
	err = json.NewDecoder(bytes.NewReader(output)).Decode(&info)
	if err == nil {
		data = info
		p.l.Debugf("Result: %#v", info)
	} else {
		p.l.Errorf("error unmarshalling: %s\n%s", err.Error(), output)
	}
	return
}

type ffprobeInfo struct {
	Streams []ffprobeStream `json:"streams"`
	Format  ffprobeFormat   `json:"format"`
}

type ffprobeStream struct {
	CodecName   string `json:"codec_name"`
	CodecType   string `json:"codec_type"`
	Width       uint   `json:"width"`
	Height      uint   `json:"height"`
	SampleRate  string `json:"sample_rate"`
	Channels    uint   `json:"channels"`
	Disposition struct {
		Default int `json:"default"`
	} `json:"disposition"`
}

type ffprobeFormat struct {
	Duration floatString       `json:"duration"`
	Size     integerString     `json:"size"`
	BitRate  string            `json:"bit_rate"`
	Tags     map[string]string `json:"tags"`
}

type floatString float64

func (s *floatString) UnmarshalText(t []byte) (err error) {
	f, err := strconv.ParseFloat(string(t), 64)
	if err == nil {
		(*s) = floatString(f)
	}
	return
}

type integerString int64

func (s *integerString) UnmarshalText(t []byte) (err error) {
	i, err := strconv.ParseInt(string(t), 10, 64)
	if err == nil {
		(*s) = integerString(i)
	}
	return
}

func formatDuration(d float64) string {
	h := uint64(d) / 3600
	m := (uint64(d) / 60) % 60
	s := math.Mod(d, 60.0)
	return fmt.Sprintf("%d:%02d:%02.6f", h, m, s)
}
