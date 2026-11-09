package master

import (
	"bufio"
	"context"
	"fmt"
	"image"
	"io"
	"log"
	"net/url"
	"orbit/internal/proto"
	"orbit/internal/screen"
	"sync"
	"time"
)

// Demo broadcasts one screen (the teacher's or a student's) to many
// students. The source produces stream frames; every target has its own
// buffered queue. A target that falls behind skips frames until the next key
// frame, so one slow computer never slows down the others.
type Demo struct {
	m       *Master
	ID      string
	Source  string // "teacher" or computer id
	Mode    string // fullscreen | window
	Targets []string
	Started time.Time
	ctx     context.Context
	cancel  context.CancelFunc

	mu     sync.Mutex
	subs   map[string]*demoSub
	keyReq chan struct{}
}

type demoMsg struct {
	data []byte
	key  bool
}

type demoSub struct {
	id      string
	ch      chan demoMsg
	waitKey bool
	state   string
}

type demoView struct {
	ID         string `json:"id"`
	Source     string `json:"source"`
	SourceName string `json:"sourceName"`
	Mode       string `json:"mode"`
	Targets    int    `json:"targets"`
	Connected  int    `json:"connected"`
	Started    int64  `json:"started"`
}

func (d *Demo) view() *demoView {
	d.mu.Lock()
	defer d.mu.Unlock()
	v := &demoView{ID: d.ID, Source: d.Source, Mode: d.Mode, Targets: len(d.Targets), Started: d.Started.Unix()}
	for _, s := range d.subs {
		if s.state == "live" {
			v.Connected++
		}
	}
	if d.Source != "teacher" {
		if pc := d.m.cfg.computer(d.Source); pc != nil {
			v.SourceName = pc.Name
		}
	}
	return v
}

func (d *Demo) requestKey() {
	select {
	case d.keyReq <- struct{}{}:
	default:
	}
}

func (d *Demo) setSubState(s *demoSub, st string) {
	d.mu.Lock()
	changed := s.state != st
	s.state = st
	d.mu.Unlock()
	if changed {
		d.m.markDirty()
	}
}

func (d *Demo) broadcast(msg demoMsg) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, s := range d.subs {
		if s.waitKey && !msg.key {
			continue
		}
		select {
		case s.ch <- msg:
			if msg.key {
				s.waitKey = false
			}
		default:
			s.waitKey = true // queue full: drop until the next key frame
			d.requestKey()
		}
	}
}

// startDemo starts a new demo (stopping a running one).
func (m *Master) startDemo(source, mode string, targets []string) error {
	m.stopDemo()
	if mode != "window" {
		mode = "fullscreen"
	}
	ctx, cancel := context.WithCancel(context.Background())
	d := &Demo{m: m, ID: proto.RandomID(), Source: source, Mode: mode, Started: time.Now(), ctx: ctx, cancel: cancel,
		subs: map[string]*demoSub{}, keyReq: make(chan struct{}, 1)}
	for _, id := range targets {
		if id == source {
			continue // never show a student their own screen
		}
		d.Targets = append(d.Targets, id)
		d.subs[id] = &demoSub{id: id, ch: make(chan demoMsg, 4), waitKey: true, state: "connecting"}
	}
	if len(d.Targets) == 0 {
		cancel()
		return fmt.Errorf("no target computers")
	}
	m.mu.Lock()
	m.demo = d
	m.mu.Unlock()
	if source == "teacher" {
		go d.teacherSource()
	} else {
		go d.studentSource(source)
	}
	for _, s := range d.subs {
		go d.pushTo(s)
	}
	log.Printf("demo %s started: source=%s mode=%s targets=%d", d.ID, source, mode, len(d.Targets))
	m.markDirty()
	return nil
}

// stopDemo stops the running demo and tells all students to close it.
func (m *Master) stopDemo() {
	m.mu.Lock()
	d := m.demo
	m.demo = nil
	m.mu.Unlock()
	if d == nil {
		return
	}
	d.cancel()
	var wg sync.WaitGroup
	for _, id := range d.Targets {
		if c := m.conn(id); c != nil {
			wg.Add(1)
			go func() { defer wg.Done(); c.Cmd(proto.Command{Action: "demo_stop"}) }()
		}
	}
	wg.Wait()
	log.Printf("demo %s stopped", d.ID)
	m.markDirty()
}

// teacherSource captures the teacher's primary screen.
func (d *Demo) teacherSource() {
	s := d.m.settings()
	enc := &proto.Encoder{Quality: s.DemoQuality}
	interval := time.Second / time.Duration(max(1, min(30, s.DemoFPS)))
	var img *image.RGBA
	lastKey := time.Now()
	for {
		t0 := time.Now()
		select {
		case <-d.ctx.Done():
			return
		default:
		}
		force := time.Since(lastKey) > 10*time.Second
		select {
		case <-d.keyReq:
			force = true
		default:
		}
		area := screen.PrimaryArea()
		w, h := screen.FitSize(area.Dx(), area.Dy(), s.DemoMaxW)
		var err error
		img, err = screen.Capture(img, area, w, h, true)
		if err == nil {
			data, key, err := enc.Encode(img, force)
			if err == nil && data != nil {
				if key {
					lastKey = time.Now()
				}
				d.broadcast(demoMsg{data, key})
			}
		}
		select {
		case <-d.ctx.Done():
			return
		case <-time.After(max(time.Millisecond, interval-time.Since(t0))):
		}
	}
}

// studentSource relays a student's live screen to the others.
func (d *Demo) studentSource(id string) {
	s := d.m.settings()
	for {
		select {
		case <-d.ctx.Done():
			return
		default:
		}
		c := d.m.conn(id)
		if c == nil || !c.online() {
			time.Sleep(time.Second)
			continue
		}
		q := url.Values{"q": {fmt.Sprint(s.DemoQuality)}, "maxw": {fmt.Sprint(s.DemoMaxW)}, "fps": {fmt.Sprint(s.DemoFPS)}, "kf": {"3"}, "purpose": {"demo"}}
		resp, err := c.do(d.ctx, c.streamClient, "GET", "/api/v1/stream?"+q.Encode(), nil, "", nil)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		br := bufio.NewReaderSize(resp.Body, 512<<10)
		for {
			f, err := proto.ReadFrame(br)
			if err != nil {
				break
			}
			d.broadcast(demoMsg{f.Raw, f.Key})
		}
		resp.Body.Close()
	}
}

// pushTo streams frames to one student computer, reconnecting as needed.
func (d *Demo) pushTo(s *demoSub) {
	for {
		select {
		case <-d.ctx.Done():
			return
		default:
		}
		c := d.m.conn(s.id)
		if c == nil || !c.online() {
			d.setSubState(s, "offline")
			time.Sleep(1500 * time.Millisecond)
			continue
		}
		pr, pw := io.Pipe()
		d.mu.Lock()
		s.waitKey = true
		d.mu.Unlock()
		d.requestKey()
		done := make(chan error, 1)
		go func() {
			resp, err := c.do(d.ctx, c.streamClient, "POST", "/api/v1/demo?mode="+d.Mode, pr, "application/x-orbit-stream", nil)
			if err == nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
			pr.CloseWithError(io.EOF)
			done <- err
		}()
		d.setSubState(s, "live")
	loop:
		for {
			select {
			case <-d.ctx.Done():
				pw.Close()
				<-done
				return
			case err := <-done:
				_ = err
				break loop
			case msg := <-s.ch:
				if _, err := pw.Write(msg.data); err != nil {
					break loop
				}
			}
		}
		pw.Close()
		d.setSubState(s, "connecting")
		time.Sleep(time.Second)
	}
}
