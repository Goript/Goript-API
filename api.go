package goript

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
)

type C struct {
	Player string
	Args   []string
}

type Script struct {
	Name      string
	Prefix    string
	Perm      string
	OpBypass  bool
	Handler   func(c C) error
	HelpLines []string
}

type msg struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

type permReq struct {
	Player     string `json:"player"`
	Permission string `json:"permission"`
	OpBypass   bool   `json:"op_bypass"`
}

var (
	stdout   = bufio.NewWriter(os.Stdout)
	stdin    = bufio.NewScanner(os.Stdin)
	permChan = make(chan bool, 1)
	mu       sync.Mutex
)

func New() *Script { return &Script{} }

func (s *Script) RegisterCommand() {
	mu.Lock()
	defer mu.Unlock()
	_ = json.NewEncoder(stdout).Encode(map[string]any{
		"type": "register_command",
		"data": map[string]string{"command": s.Name},
	})
	stdout.Flush()
}

func (s *Script) HasPermission(player string) bool {
	_ = json.NewEncoder(stdout).Encode(map[string]any{
		"type": "check_permission",
		"data": permReq{player, s.Perm, s.OpBypass},
	})
	stdout.Flush()
	return <-permChan
}

func (s *Script) SendMessage(to, text string) {
	_ = json.NewEncoder(stdout).Encode(map[string]any{
		"type": "player_action",
		"data": map[string]any{
			"action":  "send_message",
			"player":  to,
			"message": fmt.Sprintf("<color:#F5C527>%s <dark_gray>> <gray>%s", s.Prefix, text),
		},
	})
	stdout.Flush()
}

func (s *Script) MainLoop() {
	for stdin.Scan() {
		var m msg
		if err := json.Unmarshal(stdin.Bytes(), &m); err != nil {
			continue
		}
		switch m.Type {
		case "command":
			var cmd struct {
				Command string   `json:"command"`
				Player  string   `json:"player"`
				Args    []string `json:"args"`
			}
			_ = json.Unmarshal(m.Data, &cmd)
			if cmd.Command != s.Name {
				continue
			}
			if s.Perm != "" && !s.HasPermission(cmd.Player) {
				s.SendMessage(cmd.Player, "§cNo permission.")
				continue
			}
			if err := s.Handler(C{cmd.Player, cmd.Args}); err != nil {
				log.Println(err)
			}
		case "permission_result":
			var res struct {
				HasPermission bool `json:"has_permission"`
			}
			_ = json.Unmarshal(m.Data, &res)
			permChan <- res.HasPermission
		}
	}
}
