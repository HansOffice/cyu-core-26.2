package main

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

func logInfo(format string, a ...interface{}) {
	t := time.Now().Format("15:04:05")
	msg := fmt.Sprintf(format, a...)
	fmt.Printf("[%s INFO]: %s\n", t, msg)
}

func logWarn(format string, a ...interface{}) {
	t := time.Now().Format("15:04:05")
	msg := fmt.Sprintf(format, a...)
	fmt.Printf("[%s WARN]: %s\n", t, msg)
}

func logError(format string, a ...interface{}) {
	t := time.Now().Format("15:04:05")
	msg := fmt.Sprintf(format, a...)
	fmt.Printf("[%s ERROR]: %s\n", t, msg)
}

func printServerBanner() {
	banner := `
  ____ _  _ _  _    ____ ____ ____ ____ 
  |___  \/  |  |    |    |  | |__/ |___ 
  |___ _/\_ |__|    |___ |__| |  \ |___ 
`
	fmt.Print(banner)
	fmt.Println("  CyuCore 26.2 Native Game Engine (Release 26.2 / Protocol 776)")
	fmt.Println("  Pure Go Implementation running on " + runtime.GOOS + "/" + runtime.GOARCH)
	fmt.Println()
}

func printServerBoot(cfg *ServerConfig, startupDuration time.Duration) {
	logInfo("[CyuCore] CyuCore Minecraft 26.2 native server started")
	logInfo("[CyuCore] Protocol version: %d (Target: 26.2 / Fluid-Separation Architecture)", ProtocolVersion26_2)
	logInfo("[CyuCore] Spawn: %.1f, %.1f, %.1f", cfg.SpawnX, cfg.SpawnY, cfg.SpawnZ)
	logInfo("[CyuCore] TCP listener bound on *:%d", cfg.Port)
	logInfo("[CyuCore] Done (%s). For help, type \"help\"", startupDuration.Round(time.Millisecond))
}

func runConsole(server *Server, stopChan chan struct{}) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		cmd := strings.ToLower(parts[0])

		switch cmd {
		case "help", "?":
			logInfo("--- CyuCore 26.2 指令帮助 ---")
			logInfo("help                  - 查看所有可用控制台指令")
			logInfo("list                  - 查看当前真实在线玩家列表与坐标")
			logInfo("say <message>         - 向全服在线玩家广播游戏内聊天消息")
			logInfo("kick <player> [reason]- 踢出指定在线玩家")
			logInfo("tps                   - 查看当前核心真实 TPS 与微秒耗时")
			logInfo("status                - 查看服务器运行时间与内存资源")
			logInfo("stop                  - 踢出全服玩家并安全关闭服务端")
		case "list":
			players := server.ListPlayers()
			logInfo("当前在线玩家 (%d): %s", len(players), strings.Join(players, ", "))
		case "say":
			if len(parts) > 1 {
				msg := strings.Join(parts[1:], " ")
				server.BroadcastSystemMessage("&d[Server] &f" + msg)
				logInfo("[Server] %s", msg)
			} else {
				logWarn("用法: say <message>")
			}
		case "kick":
			if len(parts) > 1 {
				target := parts[1]
				reason := "&c你已被管理员移出服务器"
				if len(parts) > 2 {
					reason = "&c" + strings.Join(parts[2:], " ")
				}
				if server.KickPlayer(target, reason) {
					logInfo("成功踢出玩家: %s", target)
				} else {
					logWarn("未找到玩家: %s", target)
				}
			} else {
				logWarn("用法: kick <player> [reason]")
			}
		case "tps":
			metrics := server.TickMetrics()
			if metrics.TPS == 0 {
				logInfo("TPS: warming up / target %.2f | Ticks: %d", metrics.TargetTPS, metrics.TotalTicks)
			} else {
				logInfo("TPS: %.2f / %.2f | Ticks: %d", metrics.TPS, metrics.TargetTPS, metrics.TotalTicks)
			}
			logInfo("MSPT: %.3f last / %.3f EWMA | Budget: %.1fms", metrics.LastMSPT, metrics.AverageMSPT, 1000.0/metrics.TargetTPS)
		case "status":
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			allocMB := float64(m.Alloc) / 1024 / 1024
			sysMB := float64(m.Sys) / 1024 / 1024
			uptime := time.Since(server.startTime).Round(time.Second)

			logInfo("--- CyuCore 26.2 运行状态 ---")
			logInfo("Engine: CyuCore 26.2 Native Go Server (Protocol 776)")
			logInfo("Uptime: %s | Online: %d 玩家", uptime, server.GetOnlineCount())
			logInfo("Goroutines: %d | Memory: %.2f MB / %.2f MB", runtime.NumGoroutine(), allocMB, sysMB)
		case "stop", "end":
			logInfo("[CyuCore] 正在关闭服务端...")
			server.Stop()
			logInfo("[CyuCore] 服务端已安全退出。")
			close(stopChan)
			return
		default:
			logWarn("未知指令: %s。输入 'help' 查看帮助", cmd)
		}
	}
}
