package goript

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Context struct {
	Player string
	Args   []string
	api    *API
}

type Handler func(ctx *Context) error

type API struct {
	commands map[string]*Command
	scanner  *bufio.Scanner
	writer   *bufio.Writer
	permChan chan permResult
	mu       sync.RWMutex
}

type Command struct {
	Name        string
	Permission  string
	OpBypass    bool
	Handler     Handler
	Description string
}

type permResult struct {
	key    string
	result bool
}

type message struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

func New() *API {
	return &API{
		commands: make(map[string]*Command),
		scanner:  bufio.NewScanner(os.Stdin),
		writer:   bufio.NewWriter(os.Stdout),
		permChan: make(chan permResult, 100),
	}
}

func (api *API) Command(name, permission string, opBypass bool, handler Handler) *API {
	cmd := &Command{
		Name:       name,
		Permission: permission,
		OpBypass:   opBypass,
		Handler:    handler,
	}
	api.commands[name] = cmd
	api.registerCommand(name)
	return api
}

func (api *API) Listen() {
	go api.handlePermissions()
	
	for api.scanner.Scan() {
		var msg message
		if err := json.Unmarshal(api.scanner.Bytes(), &msg); err != nil {
			continue
		}
		
		switch msg.Type {
		case "command":
			api.handleCommand(msg.Data)
		case "permission_result":
			api.handlePermissionResult(msg.Data)
		}
	}
}

func (ctx *Context) Reply(message string) {
	ctx.api.sendPlayerMessage(ctx.Player, message)
}

func (ctx *Context) Replyf(format string, args ...interface{}) {
	ctx.Reply(fmt.Sprintf(format, args...))
}

func (ctx *Context) HasPerm(permission string) bool {
	return ctx.api.checkPermission(ctx.Player, permission, true)
}

func (ctx *Context) GetInt(index int, defaultValue int) int {
	if index >= len(ctx.Args) {
		return defaultValue
	}
	if val, err := strconv.Atoi(ctx.Args[index]); err == nil {
		return val
	}
	return defaultValue
}

func (ctx *Context) GetFloat(index int, defaultValue float64) float64 {
	if index >= len(ctx.Args) {
		return defaultValue
	}
	if val, err := strconv.ParseFloat(ctx.Args[index], 64); err == nil {
		return val
	}
	return defaultValue
}

func (ctx *Context) GetString(index int, defaultValue string) string {
	if index >= len(ctx.Args) {
		return defaultValue
	}
	return ctx.Args[index]
}

func (ctx *Context) Len() int {
	return len(ctx.Args)
}

func (ctx *Context) SetGamemode(player, gamemode string) {
	ctx.api.setGamemode(player, gamemode)
}

func (ctx *Context) Teleport(player string, x, y, z float64, world ...string) {
	w := ""
	if len(world) > 0 {
		w = world[0]
	}
	ctx.api.teleport(player, x, y, z, w)
}

func (ctx *Context) TeleportTo(player, target string) {
	ctx.api.teleportToPlayer(player, target)
}

func (ctx *Context) Heal(player string) {
	ctx.api.heal(player)
}

func (ctx *Context) Feed(player string) {
	ctx.api.feed(player)
}

func (ctx *Context) Fly(player string, enabled bool) {
	ctx.api.setFly(player, enabled)
}

func (ctx *Context) God(player string, enabled bool) {
	ctx.api.setGodMode(player, enabled)
}

func (ctx *Context) Give(player, material string, amount int) {
	ctx.api.giveItem(player, material, amount)
}

func (ctx *Context) Execute(command string) {
	ctx.api.executeCommand(command)
}

func (ctx *Context) Broadcast(message string) {
	ctx.api.broadcast(message)
}

func (ctx *Context) Usage(usage string) {
	ctx.Reply("<red>Usage: " + usage)
}

func (ctx *Context) NoPermission() {
	ctx.Reply("<red>You don't have permission to use this command!")
}

func (ctx *Context) PlayerNotFound(player string) {
	ctx.Reply("<red>Player '" + player + "' not found!")
}

func (ctx *Context) Success(message string) {
	ctx.Reply("<green>" + message)
}

func (ctx *Context) Error(message string) {
	ctx.Reply("<red>" + message)
}

func (ctx *Context) Info(message string) {
	ctx.Reply("<blue>" + message)
}

func (api *API) registerCommand(name string) {
	api.send(map[string]interface{}{
		"type": "register_command",
		"data": map[string]string{"command": name},
	})
}

func (api *API) handleCommand(data json.RawMessage) {
	var cmd struct {
		Command string   `json:"command"`
		Player  string   `json:"player"`
		Args    []string `json:"args"`
	}
	
	if err := json.Unmarshal(data, &cmd); err != nil {
		return
	}
	
	command, exists := api.commands[cmd.Command]
	if !exists {
		return
	}
	
	ctx := &Context{
		Player: cmd.Player,
		Args:   cmd.Args,
		api:    api,
	}
	
	if command.Permission != "" {
		if !api.checkPermission(cmd.Player, command.Permission, command.OpBypass) {
			ctx.NoPermission()
			return
		}
	}
	
	if err := command.Handler(ctx); err != nil {
		ctx.Error(fmt.Sprintf("Command error: %v", err))
	}
}

func (api *API) checkPermission(player, permission string, opBypass bool) bool {
	key := fmt.Sprintf("%s:%s:%v", player, permission, opBypass)
	
	api.send(map[string]interface{}{
		"type": "check_permission",
		"data": map[string]interface{}{
			"player":     player,
			"permission": permission,
			"op_bypass":  opBypass,
		},
	})
	
	timeout := time.After(200 * time.Millisecond)
	for {
		select {
		case result := <-api.permChan:
			if result.key == key {
				return result.result
			}
		case <-timeout:
			return false
		}
	}
}

func (api *API) handlePermissions() {
}

func (api *API) handlePermissionResult(data json.RawMessage) {
	var result struct {
		Player        string `json:"player"`
		Permission    string `json:"permission"`
		OpBypass      bool   `json:"op_bypass"`
		HasPermission bool   `json:"has_permission"`
	}
	
	if err := json.Unmarshal(data, &result); err != nil {
		return
	}
	
	key := fmt.Sprintf("%s:%s:%v", result.Player, result.Permission, result.OpBypass)
	
	select {
	case api.permChan <- permResult{key: key, result: result.HasPermission}:
	default:
	}
}

func (api *API) sendPlayerMessage(player, message string) {
	api.send(map[string]interface{}{
		"type": "player_action",
		"action": "send_message",
		"player": player,
		"data": map[string]interface{}{
			"message": message,
		},
	})
}

func (api *API) setGamemode(player, gamemode string) {
	switch strings.ToLower(gamemode) {
	case "0", "survival", "s":
		gamemode = "SURVIVAL"
	case "1", "creative", "c":
		gamemode = "CREATIVE"
	case "2", "adventure", "a":
		gamemode = "ADVENTURE"
	case "3", "spectator", "sp":
		gamemode = "SPECTATOR"
	}
	
	api.send(map[string]interface{}{
		"type": "player_action",
		"action": "set_gamemode",
		"player": player,
		"data": map[string]interface{}{
			"gamemode": gamemode,
		},
	})
}

func (api *API) teleport(player string, x, y, z float64, world string) {
	data := map[string]interface{}{
		"x": x,
		"y": y,
		"z": z,
	}
	if world != "" {
		data["world"] = world
	}
	
	api.send(map[string]interface{}{
		"type": "player_action",
		"action": "teleport",
		"player": player,
		"data": data,
	})
}

func (api *API) teleportToPlayer(player, target string) {
	api.send(map[string]interface{}{
		"type": "player_action",
		"action": "teleport_to_player",
		"player": player,
		"data": map[string]interface{}{
			"target": target,
		},
	})
}

func (api *API) heal(player string) {
	api.send(map[string]interface{}{
		"type": "player_action",
		"action": "heal",
		"player": player,
		"data": map[string]interface{}{},
	})
}

func (api *API) feed(player string) {
	api.send(map[string]interface{}{
		"type": "player_action",
		"action": "feed",
		"player": player,
		"data": map[string]interface{}{},
	})
}

func (api *API) setFly(player string, enabled bool) {
	api.send(map[string]interface{}{
		"type": "player_action",
		"action": "set_fly",
		"player": player,
		"data": map[string]interface{}{
			"enabled": enabled,
		},
	})
}

func (api *API) setGodMode(player string, enabled bool) {
	api.send(map[string]interface{}{
		"type": "player_action",
		"action": "set_god_mode",
		"player": player,
		"data": map[string]interface{}{
			"enabled": enabled,
		},
	})
}

func (api *API) giveItem(player, material string, amount int) {
	api.send(map[string]interface{}{
		"type": "player_action",
		"action": "give_item",
		"player": player,
		"data": map[string]interface{}{
			"material": strings.ToUpper(material),
			"amount":   amount,
		},
	})
}

func (api *API) executeCommand(command string) {
	api.send(map[string]interface{}{
		"type":    "execute",
		"command": command,
	})
}

func (api *API) broadcast(message string) {
	api.send(map[string]interface{}{
		"type":    "broadcast",
		"message": message,
	})
}

func (api *API) send(data interface{}) {
	api.mu.Lock()
	defer api.mu.Unlock()
	
	json.NewEncoder(api.writer).Encode(data)
	api.writer.Flush()
}
