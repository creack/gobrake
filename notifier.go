package gobrake

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/golang/glog"
	"github.com/mreiferson/go-httpclient"
)

var (
	transport = &httpclient.Transport{
		ConnectTimeout:        1 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		RequestTimeout:        10 * time.Second,
	}
	client = &http.Client{Transport: transport}
)

type Notifier struct {
	Client      *http.Client
	StackFilter func(string, int, string, string) bool

	createNoticeURL string
	context         map[string]string
}

func NewNotifier(projectId int64, key string) *Notifier {
	n := &Notifier{
		Client:      client,
		StackFilter: stackFilter,

		createNoticeURL: getCreateNoticeURL(projectId, key),
		context:         make(map[string]string),
	}
	n.context["language"] = runtime.Version()
	n.context["os"] = runtime.GOOS
	n.context["architecture"] = runtime.GOARCH
	if hostname, err := os.Hostname(); err == nil {
		n.context["hostname"] = hostname
	}
	if wd, err := os.Getwd(); err == nil {
		n.context["rootDirectory"] = wd
	}
	return n
}

func (n *Notifier) SetContext(name, value string) {
	n.context[name] = value
}

func (n *Notifier) Notify(e interface{}, req *http.Request) error {
	notice := n.Notice(e, req, 3)
	if err := n.SendNotice(notice); err != nil {
		glog.Errorf("gobrake failed (%s) reporting error: %v", err, e)
		return err
	}
	return nil
}

func (n *Notifier) Notice(e interface{}, req *http.Request, startFrame int) *Notice {
	stack := stack(startFrame, n.StackFilter)
	notice := NewNotice(e, stack, req)
	for k, v := range n.context {
		notice.Context[k] = v
	}
	return notice
}

func (n *Notifier) SendNotice(notice *Notice) error {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	if err := enc.Encode(notice); err != nil {
		return err
	}

	resp, err := n.Client.Post(n.createNoticeURL, "application/json", buf)
	if err != nil {
		return err
	}

	// Read response so underlying connection can be reused.
	io.Copy(ioutil.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf(
			"gobrake: got %d response, wanted 201", resp.StatusCode)
	}

	return nil
}
