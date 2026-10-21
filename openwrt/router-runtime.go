package main

import (
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// On Linux Go's monotonic clock excludes system suspend. A sleeping phone
// must not be mistaken for a stuck router loop by a wall-clock watchdog.
var controlStarted = time.Now()

func controlUptime() int64 { return time.Since(controlStarted).Nanoseconds() }

func heartbeatStalled(last, now int64) bool {
	return last > 0 && now-last > int64(180*time.Second)
}

const routerWakeName = "droidwrt-router"
const routerWakeLease = 30 * time.Second

var routerWakeHeld atomic.Bool

func needsRouterWake(c SharingConfig) bool {
	return c.Hotspot || c.EthernetLAN || (c.USB != "" && c.USB != "off")
}

// Leases expire even if the controller is killed. Only this named wake source
// is touched; display, CPU governors, thermal limits and other locks are intact.
func setRouterWake(enabled bool, write func(string, []byte) error) error {
	if enabled {
		return write("/sys/power/wake_lock", []byte(routerWakeName+" "+strconv.FormatInt(int64(routerWakeLease), 10)))
	}
	return write("/sys/power/wake_unlock", []byte(routerWakeName))
}

func routerPowerMonitor() {
	previous := ""
	for {
		wanted := needsRouterWake(readSharing())
		held := routerWakeHeld.Load()
		state := "idle"
		if wanted || held {
			err := setRouterWake(wanted, func(path string, data []byte) error {
				// Never create a regular file when the kernel ABI is unavailable.
				f, err := os.OpenFile(path, os.O_WRONLY, 0)
				if err != nil {
					return err
				}
				defer f.Close()
				_, err = f.Write(data)
				return err
			})
			if err != nil {
				state = "unavailable"
				routerWakeHeld.Store(false)
			} else {
				routerWakeHeld.Store(wanted)
				if wanted {
					state = "held"
				}
			}
		}
		if state != previous {
			recordHealthEvent("power_guard", map[string]any{"state": state, "wanted": wanted})
			previous = state
		}
		time.Sleep(5 * time.Second)
	}
}

func bootUptimeMillis() any {
	b, err := os.ReadFile("/proc/uptime")
	fields := strings.Fields(string(b))
	if err != nil || len(fields) == 0 {
		return nil
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return nil
	}
	return int64(seconds * 1000)
}
