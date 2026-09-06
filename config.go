package main

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"sync"
)

type ServerConfig struct {
	Port          int
	MaxPlayers    int
	VersionName   string
	MotdLine1     string
	MotdLine2     string
	SpawnX        float64
	SpawnY        float64
	SpawnZ        float64
	GameMode      int
	PlatformBlock int
}

type ConfigManager struct {
	path string
	mu   sync.RWMutex
	cfg  *ServerConfig
}

const defaultServerConfigYAML = `# cyu-core 26.2 原生游戏服务端配置

server:
  # 服务端监听端口
  # 默认 25577，局域网或公网直接连接此端口
  port: 25577

  # 最大允许在线玩家数
  max_players: 100

  # 客户端服务器列表中展示的核心版本标识
  version_name: "&bCyuCore &726.2"

motd:
  # 客户端列表第一行 MOTD
  line1: "&b&lCyuCore 26.2 &8| &7Go 原生全自研游戏核心"
  # 客户端列表第二行 MOTD
  line2: "&a✔ 原生游戏世界就绪 &8› &eTPS 20.0 &8› &f直连进入"

world:
  # 玩家默认出生点坐标
  spawn_x: 8.5
  spawn_y: 65.0
  spawn_z: 8.5

  # 玩家默认游戏模式（0: 生存模式, 1: 创造模式）
  gamemode: 1

  # 出生点悬空平台的方块类型 ID（1 为石头，也可以是其他方块）
  platform_block: 1
`

func NewConfigManager(path string) *ConfigManager {
	return &ConfigManager{path: path}
}

func (cm *ConfigManager) Load() (*ServerConfig, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if _, err := os.Stat(cm.path); os.IsNotExist(err) {
		if err := os.WriteFile(cm.path, []byte(defaultServerConfigYAML), 0644); err != nil {
			return nil, err
		}
	}

	data, err := os.ReadFile(cm.path)
	if err != nil {
		return nil, err
	}

	cm.cfg = parseServerConfig(string(data))
	return cm.cfg, nil
}

func (cm *ConfigManager) Get() *ServerConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.cfg
}

func parseServerConfig(text string) *ServerConfig {
	cfg := &ServerConfig{
		Port:          25577,
		MaxPlayers:    100,
		VersionName:   "&bCyuCore &726.2",
		MotdLine1:     "&b&lCyuCore 26.2 &8| &7Go 原生全自研游戏核心",
		MotdLine2:     "&a✔ 原生游戏世界就绪 &8› &eTPS 20.0 &8› &f直连进入",
		SpawnX:        8.5,
		SpawnY:        65.0,
		SpawnZ:        8.5,
		GameMode:      1,
		PlatformBlock: 1,
	}

	scanner := bufio.NewScanner(strings.NewReader(text))
	var section string

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && strings.HasSuffix(trimmed, ":") {
			section = strings.TrimSuffix(trimmed, ":")
			continue
		}

		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := cleanQuote(strings.TrimSpace(parts[1]))

		switch section {
		case "server":
			switch key {
			case "port":
				if p, err := strconv.Atoi(val); err == nil {
					cfg.Port = p
				}
			case "max_players":
				if m, err := strconv.Atoi(val); err == nil {
					cfg.MaxPlayers = m
				}
			case "version_name":
				cfg.VersionName = val
			}
		case "motd":
			switch key {
			case "line1":
				cfg.MotdLine1 = val
			case "line2":
				cfg.MotdLine2 = val
			}
		case "world":
			switch key {
			case "spawn_x":
				if f, err := strconv.ParseFloat(val, 64); err == nil {
					cfg.SpawnX = f
				}
			case "spawn_y":
				if f, err := strconv.ParseFloat(val, 64); err == nil {
					cfg.SpawnY = f
				}
			case "spawn_z":
				if f, err := strconv.ParseFloat(val, 64); err == nil {
					cfg.SpawnZ = f
				}
			case "gamemode":
				if gm, err := strconv.Atoi(val); err == nil {
					cfg.GameMode = gm
				}
			case "platform_block":
				if pb, err := strconv.Atoi(val); err == nil {
					cfg.PlatformBlock = pb
				}
			}
		}
	}
	return cfg
}

func cleanQuote(s string) string {
	s = strings.TrimSpace(s)
	if (strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"")) ||
		(strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'")) {
		if len(s) >= 2 {
			return s[1 : len(s)-1]
		}
	}
	return s
}
