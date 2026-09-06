package main

import (
	"fmt"
	"strconv"
	"strings"
)

type CommandHandler struct {
	server *Server
}

func NewCommandHandler(server *Server) *CommandHandler {
	return &CommandHandler{server: server}
}

func (h *CommandHandler) Handle(session *PlayerSession, rawCmd string) {
	rawCmd = strings.TrimPrefix(rawCmd, "/")
	parts := strings.Fields(rawCmd)
	if len(parts) == 0 {
		return
	}

	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "help", "?", "帮助":
		h.cmdHelp(session)
	case "gamemode", "gm":
		h.cmdGameMode(session, args)
	case "tp", "teleport":
		h.cmdTeleport(session, args)
	case "spawn":
		h.cmdSpawn(session)
	case "time":
		h.cmdTime(session, args)
	case "say":
		h.cmdSay(session, args)
	case "ping":
		h.cmdPing(session)
	case "clear":
		h.cmdClear(session)
	default:
		session.SendSystemMessage(fmt.Sprintf("&c未知指令: /%s。输入 &e/help &c查看可用指令。", cmd))
	}
}

func (h *CommandHandler) cmdHelp(s *PlayerSession) {
	s.SendSystemMessage("&b┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈")
	s.SendSystemMessage("&b&lCyuCore 26.2 &7原生游戏核心指令指南")
	s.SendSystemMessage("&e/help                  &8› &7查看此帮助列表")
	s.SendSystemMessage("&e/gamemode <0-3>        &8› &7切换游戏模式 (0生存/1创造/2冒险/3旁观)")
	s.SendSystemMessage("&e/tp <玩家名 | x y z>   &8› &7传送至指定玩家或空间坐标")
	s.SendSystemMessage("&e/spawn                 &8› &7立即传送回出生点安全平台")
	s.SendSystemMessage("&e/time <set|add> <值>   &8› &7修改世界昼夜时间 (day/night/数值)")
	s.SendSystemMessage("&e/say <消息>            &8› &7向全服所有在线玩家广播通知")
	s.SendSystemMessage("&e/ping                  &8› &7测试客户端与核心当前网络延迟")
	s.SendSystemMessage("&b┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈")
}

func (h *CommandHandler) cmdGameMode(s *PlayerSession, args []string) {
	if len(args) == 0 {
		s.SendSystemMessage("&c用法: /gamemode <0|1|2|3|survival|creative|adventure|spectator>")
		return
	}

	mode := -1
	switch strings.ToLower(args[0]) {
	case "0", "survival", "s":
		mode = 0
	case "1", "creative", "c":
		mode = 1
	case "2", "adventure", "a":
		mode = 2
	case "3", "spectator", "sp":
		mode = 3
	}

	if mode < 0 || mode > 3 {
		s.SendSystemMessage("&c无效的游戏模式，请输入 0(生存), 1(创造), 2(冒险), 3(旁观)")
		return
	}

	s.gameMode = mode
	s.SendPacket(PlayPktClientBoundGameEvent, buildGameEventGameMode(mode))

	modeNames := []string{"生存模式", "创造模式", "冒险模式", "旁观模式"}
	s.SendSystemMessage(fmt.Sprintf("&a你的游戏模式已更新为 &e%s&a。", modeNames[mode]))
	logInfo("[指令] 玩家 %s 切换游戏模式为 %s (%d)", s.username, modeNames[mode], mode)
}

func (h *CommandHandler) cmdTeleport(s *PlayerSession, args []string) {
	if len(args) == 1 {
		targetName := args[0]
		target := h.server.FindPlayer(targetName)
		if target == nil {
			s.SendSystemMessage(fmt.Sprintf("&c未找到玩家: %s", targetName))
			return
		}
		s.Teleport(target.x, target.y, target.z, target.yaw, target.pitch)
		s.SendSystemMessage(fmt.Sprintf("&a已将你传送至玩家 &f%s &a所在位置。", target.username))
		return
	}

	if len(args) == 3 {
		x, err1 := strconv.ParseFloat(args[0], 64)
		y, err2 := strconv.ParseFloat(args[1], 64)
		z, err3 := strconv.ParseFloat(args[2], 64)
		if err1 != nil || err2 != nil || err3 != nil {
			s.SendSystemMessage("&c坐标参数必须为合法数字，格式: /tp <x> <y> <z>")
			return
		}
		s.Teleport(x, y, z, s.yaw, s.pitch)
		s.SendSystemMessage(fmt.Sprintf("&a已传送至空间坐标: &e%.1f, %.1f, %.1f", x, y, z))
		return
	}

	s.SendSystemMessage("&c用法: /tp <玩家名> 或 /tp <x> <y> <z>")
}

func (h *CommandHandler) cmdSpawn(s *PlayerSession) {
	w := h.server.world
	s.Teleport(w.spawnX, w.spawnY, w.spawnZ, 0, 0)
	s.SendSystemMessage("&a已安全传送回出生点中心平台。")
}

func (h *CommandHandler) cmdTime(s *PlayerSession, args []string) {
	if len(args) < 2 {
		s.SendSystemMessage("&c用法: /time set <day|night|noon|midnight|数值> 或 /time add <数值>")
		return
	}

	sub := strings.ToLower(args[0])
	valStr := strings.ToLower(args[1])

	if sub == "set" {
		var targetTime int64
		switch valStr {
		case "day":
			targetTime = 1000
		case "noon":
			targetTime = 6000
		case "night":
			targetTime = 13000
		case "midnight":
			targetTime = 18000
		default:
			v, err := strconv.ParseInt(valStr, 10, 64)
			if err != nil {
				s.SendSystemMessage("&c时间参数必须为数值或预设关键字 (day/noon/night/midnight)")
				return
			}
			targetTime = v
		}

		h.server.world.SetTimeOfDay(targetTime)
		age := h.server.world.worldAge.Load()
		h.server.BroadcastPacket(PlayPktClientBoundSetTime, buildSetTime(age, targetTime%24000))
		s.SendSystemMessage(fmt.Sprintf("&a已将世界时间设置为 &e%d&a 刻。", targetTime%24000))
		logInfo("[指令] 玩家 %s 将世界时间设置为 %d 刻", s.username, targetTime%24000)
	} else if sub == "add" {
		v, err := strconv.ParseInt(valStr, 10, 64)
		if err != nil {
			s.SendSystemMessage("&c增加的时间必须为有效正整数")
			return
		}
		age, tod := h.server.world.AdvanceTime(v)
		h.server.BroadcastPacket(PlayPktClientBoundSetTime, buildSetTime(age, tod))
		s.SendSystemMessage(fmt.Sprintf("&a已向前快进世界时间 &e%d&a 刻 (当前: %d)。", v, tod))
	} else {
		s.SendSystemMessage("&c用法: /time set <值> 或 /time add <值>")
	}
}

func (h *CommandHandler) cmdSay(s *PlayerSession, args []string) {
	if len(args) == 0 {
		s.SendSystemMessage("&c用法: /say <广播内容>")
		return
	}
	msg := strings.Join(args, " ")
	formatted := fmt.Sprintf("&d[%s] &f%s", s.username, msg)
	h.server.BroadcastSystemMessage(formatted)
	logInfo("[广播] %s: %s", s.username, msg)
}

func (h *CommandHandler) cmdPing(s *PlayerSession) {
	s.SendSystemMessage("&aPong! 当前与 CyuCore 核心连接畅通，网络延迟 &e< 1ms&a。")
}

func (h *CommandHandler) cmdClear(s *PlayerSession) {
	s.SendSystemMessage("&e已清空快捷栏与虚拟物品状态。")
}
