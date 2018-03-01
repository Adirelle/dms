package processor

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/anacrolix/dms/cds"
	"github.com/anacrolix/dms/didl_lite"
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
		p.cache = ffprobeCache{
			gcache.New(1000).ARC().Expiration(time.Minute).LoaderFunc(p.doProbe).Build(),
			concurrencyLock(make(chan struct{}, 20)),
		}
	}
	return
}

func (p *FFProbeProcessor) Process(obj *cds.Object) {
	mType := obj.MimeType.Type
	if !(mType == "audio" || mType == "video" || mType == "image") {
		return
	}

	group := sync.WaitGroup{}
	group.Add(1 + len(obj.Resources))

	go func() {
		defer group.Done()
		p.probeObject(obj)
	}()

	for i := range obj.Resources {
		go func(res *cds.Resource) {
			defer group.Done()
			p.probeResource(mType, res)
		}(&obj.Resources[i])
	}

	group.Wait()
}

var tagMap = map[string]string{
	"artist": didl_lite.TagArtist,
	"album":  didl_lite.TagAlbum,
	"genre":  didl_lite.TagGenre,
}

func (p *FFProbeProcessor) probeObject(obj *cds.Object) {
	info, err := p.probePath(obj.FilePath)
	if err != nil {
		return
	}

	if title, ok := info.Format.Tags["title"]; ok {
		obj.Title = title
	}

	if createdStr, ok := info.Format.Tags["creation_time"]; ok {
		created, err := time.Parse(time.RFC3339Nano, createdStr)
		if err == nil {
			obj.Tags[didl_lite.TagDate] = created.Format(time.RFC3339)
		}
	}

	for tagName, attrName := range tagMap {
		if value, ok := info.Format.Tags[tagName]; ok {
			obj.Tags[attrName] = value
		}
	}
}

func (p *FFProbeProcessor) probeResource(mainType string, res *cds.Resource) {
	info, err := p.probePath(res.FilePath)
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
		res.SetTag(didl_lite.ResBitrate, info.Format.BitRate)
		if info.Format.Duration != 0.0 {
			duration := float64(info.Format.Duration) * float64(time.Second)
			res.SetTag(didl_lite.ResDuration, didl_lite.Duration(time.Duration(duration)))
		}
	}

	gotVideo, gotAudio := false, false
	for _, s := range info.Streams {
		if hasVideo && s.CodecType == "video" && (!gotVideo || s.Disposition.Default == 1) {
			gotVideo = true
			if s.Width != 0 && s.Height != 0 {
				res.SetTag(didl_lite.ResResolution, didl_lite.Resolution{s.Width, s.Height})
			}
		}
		if hasAudio && s.CodecType == "audio" && (!gotAudio || s.Disposition.Default == 1) {
			gotAudio = true
			if s.SampleRate != 0 {
				res.SetTag(didl_lite.ResSampleFrequency, s.SampleRate)
			}
			if s.Channels != 0 {
				res.SetTag(didl_lite.ResNrAudioChannels, s.Channels)
			}
		}
	}
}

func (p *FFProbeProcessor) probePath(path string) (ffprobeInfo, error) {
	return p.cache.Get(path)
}

type ffprobeCache struct {
	gcache.Cache
	sync.Locker
}

func (c ffprobeCache) Get(path string) (ffprobeInfo, error) {
	v, err := c.Cache.GetIFPresent(path)
	if v != nil {
		return v.(ffprobeInfo), nil
	} else if err != gcache.KeyNotFoundError {
		return ffprobeInfo{}, err
	}
	c.Lock()
	defer c.Unlock()
	v, err = c.Cache.Get(path)
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
