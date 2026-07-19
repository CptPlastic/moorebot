package botcfg

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type Motion struct {
	VIO   int `json:"VIO"`
	Track int `json:"track"` // 1=tracked, 0=mecanum
}

type Client struct {
	Host     string
	User     string
	Password string
}

func (c Client) dial() (*ssh.Client, error) {
	cfg := &ssh.ClientConfig{
		User:            c.User,
		Auth:            []ssh.AuthMethod{ssh.Password(c.Password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         8 * time.Second,
	}
	addr := c.Host
	if !strings.Contains(addr, ":") {
		addr += ":22"
	}
	return ssh.Dial("tcp", addr, cfg)
}

func (c Client) run(cmd string) (string, error) {
	cli, err := c.dial()
	if err != nil {
		return "", err
	}
	defer cli.Close()
	s, err := cli.NewSession()
	if err != nil {
		return "", err
	}
	defer s.Close()
	out, err := s.CombinedOutput(cmd)
	return string(out), err
}

func (c Client) ReadMotion() (Motion, error) {
	out, err := c.run("sudo -n cat /var/roller_eye/config/motion")
	if err != nil {
		return Motion{}, fmt.Errorf("read motion: %w (%s)", err, out)
	}
	var m Motion
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &m); err != nil {
		return Motion{}, err
	}
	return m, nil
}

func (c Client) SetTrack(tracked bool) (Motion, error) {
	m, err := c.ReadMotion()
	if err != nil {
		m = Motion{VIO: 0, Track: 1}
	}
	if tracked {
		m.Track = 1
	} else {
		m.Track = 0
	}
	m.VIO = 0
	b, err := json.Marshal(m)
	if err != nil {
		return m, err
	}
	b64 := base64.StdEncoding.EncodeToString(b)
	// Write only — do not pkill in the same SSH session. pkill -f matching the
	// path string on this command line SIGTERMs the session (exit 143).
	writeCmd := fmt.Sprintf(
		`sudo -n python -c "import base64; open('/var/roller_eye/config/motion','wb').write(base64.b64decode('%s'))"`,
		b64,
	)
	if out, err := c.run(writeCmd); err != nil {
		return m, fmt.Errorf("set track write: %w (%s)", err, strings.TrimSpace(out))
	}
	// Bounce motor_node in a separate session. [/] avoids matching this cmdline.
	_, _ = c.run(`sudo -n pkill -f '[/]roller_eye/motor_node' >/dev/null 2>&1 || true`)
	got, err := c.ReadMotion()
	if err != nil {
		return m, fmt.Errorf("set track verify: %w", err)
	}
	return got, nil
}
