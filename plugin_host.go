package main

import (
	"fmt"

	commandapi "cyu-core-26.2/api/command"
	eventapi "cyu-core-26.2/api/event"
	playerapi "cyu-core-26.2/api/player"
	pluginapi "cyu-core-26.2/api/plugin"
	"cyu-core-26.2/internal/pluginruntime"
)

const maxPluginTasksPerTick = 256

var coreReservedPluginCommands = []commandapi.Name{
	"help",
	"gamemode",
	"gm",
	"tp",
	"teleport",
	"spawn",
	"time",
	"say",
	"ping",
	"clear",
}

// RegisterPlugin is a host-side registration boundary used by future loaders
// and built-in integration tests. Plugins themselves never receive *Server.
func (s *Server) RegisterPlugin(candidate pluginapi.Plugin) error {
	if s == nil || s.plugins == nil {
		return fmt.Errorf("server: plugin runtime unavailable")
	}
	return s.plugins.Register(candidate)
}

func (s *Server) PluginSnapshots() []pluginruntime.Snapshot {
	if s == nil || s.plugins == nil {
		return nil
	}
	return s.plugins.Snapshots()
}

func (s *Server) dispatchPluginEvent(event eventapi.Event) {
	if s == nil || s.plugins == nil || event == nil {
		return
	}
	for _, err := range s.plugins.Dispatch(event) {
		logWarn("[plugin] event dispatch error: %v", err)
	}
}

func (s *Server) runPluginScheduler() {
	if s == nil || s.plugins == nil {
		return
	}
	for _, err := range s.plugins.AdvanceTick(maxPluginTasksPerTick) {
		logWarn("[plugin] scheduled task error: %v", err)
	}
}

func (s *Server) executePluginCommand(session *PlayerSession, name string, args []string) bool {
	if s == nil || s.plugins == nil || session == nil {
		return false
	}
	commandName, err := commandapi.ParseName(name)
	if err != nil {
		return false
	}

	handled, err := s.plugins.ExecuteCommand(commandName, args, playerPluginCommandSource{server: s, session: session})
	if err != nil {
		logWarn("[plugin] command %s failed: %v", commandName, err)
		session.SendSystemMessage("&c插件命令执行失败，请查看服务端日志。")
		return true
	}
	return handled
}

func (s *Server) pluginPlayerSnapshot(session *PlayerSession) playerapi.Snapshot {
	if session == nil {
		return playerapi.Snapshot{}
	}
	state := session.readPlayerSnapshot()
	return playerapi.Snapshot{
		UUID:     playerapi.UUID(session.uuid),
		Name:     session.username,
		GameMode: playerapi.GameMode(state.gameMode),
		Position: playerapi.Position{
			X:        state.x,
			Y:        state.y,
			Z:        state.z,
			Yaw:      state.yaw,
			Pitch:    state.pitch,
			OnGround: state.onGround,
		},
	}
}

type playerPluginCommandSource struct {
	server  *Server
	session *PlayerSession
}

func (playerPluginCommandSource) Kind() commandapi.SourceKind { return commandapi.SourcePlayer }

func (s playerPluginCommandSource) Player() (playerapi.Snapshot, bool) {
	if s.server == nil || s.session == nil {
		return playerapi.Snapshot{}, false
	}
	return s.server.pluginPlayerSnapshot(s.session), true
}

func (s playerPluginCommandSource) Reply(message string) {
	if s.session != nil {
		s.session.SendSystemMessage(message)
	}
}

type corePluginLogger struct {
	id pluginapi.ID
}

func (l corePluginLogger) Debug(message string) { logInfo("[plugin:%s DEBUG] %s", l.id, message) }
func (l corePluginLogger) Info(message string)  { logInfo("[plugin:%s] %s", l.id, message) }
func (l corePluginLogger) Warn(message string)  { logWarn("[plugin:%s] %s", l.id, message) }
func (l corePluginLogger) Error(message string) { logError("[plugin:%s] %s", l.id, message) }

func newPluginManager() *pluginruntime.Manager {
	manager := pluginruntime.New(func(descriptor pluginapi.Descriptor) pluginapi.Logger {
		return corePluginLogger{id: descriptor.ID}
	})
	if err := manager.ReserveCommands(coreReservedPluginCommands...); err != nil {
		panic(fmt.Sprintf("invalid core command reservation: %v", err))
	}
	return manager
}
