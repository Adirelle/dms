package ffprobe

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"sync"
	"time"

	"github.com/Adirelle/dms/pkg/filesystem"

	"github.com/Adirelle/dms/pkg/cache"
	"github.com/Adirelle/dms/pkg/cds"
	"github.com/Adirelle/go-libs/logging"
)

type Config struct {
	BinPath string `json:"binPath"`
	Limit   uint   `json:"limit"`
}

type Processor struct {
	binPath string
	l       logging.Logger
	m       cache.Memo
	lk      sync.Locker
}

func (Processor) String() string {
	return "FFProbeProcessor"
}

func NewProcessor(c Config, cm *cache.Manager, l logging.Logger) (p *Processor, err error) {
	realPath, err := exec.LookPath(c.BinPath)
	if err != nil {
		return
	}
	p = &Processor{binPath: realPath,
		l:  l,
		lk: concurrencyLock(make(chan struct{}, c.Limit)),
	}
	p.m = cm.NewMemo("ffprobe", Info{}, p.loader)
	return
}

func (p *Processor) Process(obj *cds.Object, ctx context.Context) {
	t := obj.MimeType.Type
	if !(t == "audio" || t == "video" || t == "image") {
		return
	}

	l := logging.MustFromContext(ctx)

	wg := sync.WaitGroup{}
	wg.Add(1 + len(obj.Resources))

	go func() {
		defer wg.Done()
		if err := p.probeObject(obj, ctx); err != nil {
			l.Error(err)
		}
	}()

	for i := range obj.Resources {
		go func(res *cds.Resource) {
			defer wg.Done()
			if err := p.probeResource(res, ctx); err != nil {
				l.Error(err)
			}
		}(&obj.Resources[i])
	}

	wg.Wait()
}

func (p *Processor) probeObject(obj *cds.Object, ctx context.Context) error {
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

func (p *Processor) probeResource(res *cds.Resource, ctx context.Context) error {
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

func (p *Processor) probePath(path string, ctx context.Context) (*Info, error) {
	select {
	case res := <-p.m.Get(path):
		return res.(*Info), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (p *Processor) loader(key interface{}) (value interface{}, err error) {
	p.lk.Lock()
	defer p.lk.Unlock()

	filePath := key.(string)
	l := p.l.With("path", filePath)
	fi, err := filesystem.ItemFromPath(filePath)
	if err != nil {
		return
	}

	cmd := exec.Command(p.binPath, "-i", filePath, "-of", "json", "-v", "error", "-show_format", "-show_streams")

	l.Debugf("running %v", cmd.Args)
	output, err := cmd.Output()
	if err != nil {
		return
	}

	info := &Info{FileItem: fi}
	err = json.NewDecoder(bytes.NewReader(output)).Decode(info)
	if err != nil {
		l.Errorf("error unmarshalling: %s\n%s", err.Error(), output)
		return
	}

	return info, nil
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
