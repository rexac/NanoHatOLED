package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	nanohatoled "nanohat-oled/ext"

	"golang.org/x/sys/unix"
)

const (
	logFilePath   = "/tmp/nanohat-oled.log"
	pidFilePath   = "/var/run/nanohat-oled.pid"
	logoPath      = "/etc/NanoHatOLED/logo.png"
	pageSleep     = 10
	displayWidth  = 128
	displayHeight = 64
	btnK1         = 0
	btnK2         = 1
	btnK3         = 2
	timeX         = 8
	timeY         = 38
	timeWidth     = 110
	timeHeight    = 28
)

var (
	logger          *LocalTimeLogger
	oled            *nanohatoled.NanoOled
	pageIndex       int
	pageSleepCount  int
	drawing         bool
	pageMutex       sync.Mutex
	shutdownFlag    bool
	shutdownSelect  int
	lastPageIndex   int
	lastTimeStr     string
	lastShutdownSel int
	staticDrawn     bool
	localLoc        *time.Location
)

// executeDateCommand runs date command with specified argument via syscall.Exec
func executeDateCommand(arg string) (string, error) {
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		return "", fmt.Errorf("pipe creation failed: %v", err)
	}
	defer readPipe.Close()

	pid, err := syscall.ForkExec(
		"/bin/date",
		[]string{"date", arg},
		&syscall.ProcAttr{
			Env: os.Environ(),
			Files: []uintptr{
				os.Stdin.Fd(),
				writePipe.Fd(),
				os.Stderr.Fd(),
			},
		},
	)
	if err != nil {
		writePipe.Close()
		return "", fmt.Errorf("fork failed: %v", err)
	}

	writePipe.Close()

	output, err := io.ReadAll(readPipe)
	if err != nil {
		return "", fmt.Errorf("read pipe failed: %v", err)
	}

	var waitStatus syscall.WaitStatus
	_, err = syscall.Wait4(pid, &waitStatus, 0, nil)
	if err != nil {
		return "", fmt.Errorf("wait for child failed: %v", err)
	}

	if !waitStatus.Exited() || waitStatus.ExitStatus() != 0 {
		return "", fmt.Errorf("date command failed (exit code: %d)", waitStatus.ExitStatus())
	}

	return strings.TrimSpace(string(output)), nil
}

// initLocalLocation retrieves and caches system local timezone
func initLocalLocation() {
	tzOffsetStr, err := executeDateCommand("+%z")
	if err != nil || len(tzOffsetStr) != 5 {
		localtimePath := "/etc/localtime"
		if link, err := os.Readlink(localtimePath); err == nil {
			tzParts := strings.Split(link, "/zoneinfo/")
			if len(tzParts) == 2 {
				if loc, err := time.LoadLocation(tzParts[1]); err == nil {
					localLoc = loc
					return
				}
			}
		}

		if loc, err := time.LoadLocation("Local"); err == nil {
			localLoc = loc
			fmt.Printf("Failed to get timezone offset, using system default timezone: %s\n", loc.String())
			return
		}

		localLoc = time.UTC
		fmt.Printf("Failed to get local timezone, falling back to UTC\n")
		return
	}

	sign := tzOffsetStr[0]
	hourStr := tzOffsetStr[1:3]
	minStr := tzOffsetStr[3:5]

	hour, _ := strconv.Atoi(hourStr)
	min, _ := strconv.Atoi(minStr)

	offsetSec := hour*3600 + min*60
	if sign == '-' {
		offsetSec = -offsetSec
	}

	tzAbbr, err := executeDateCommand("+%Z")
	if err != nil {
		tzAbbr = "LOCAL"
	}

	localLoc = time.FixedZone(tzAbbr, offsetSec)
}

// LocalTimeLogger adds local timezone timestamps to logs
type LocalTimeLogger struct {
	base *log.Logger
}

func (l *LocalTimeLogger) Println(v ...interface{}) {
	now := time.Now().In(localLoc)
	timeStr := now.Format("2006/01/02 15:04:05")
	prefix := fmt.Sprintf("[NanoHatOLED] %s ", timeStr)
	l.base.SetPrefix(prefix)
	l.base.Println(v...)
}

func (l *LocalTimeLogger) Printf(format string, v ...interface{}) {
	now := time.Now().In(localLoc)
	timeStr := now.Format("2006/01/02 15:04:05")
	prefix := fmt.Sprintf("[NanoHatOLED] %s ", timeStr)
	l.base.SetPrefix(prefix)
	l.base.Printf(format, v...)
}

func (l *LocalTimeLogger) Fatalf(format string, v ...interface{}) {
	now := time.Now().In(localLoc)
	timeStr := now.Format("2006/01/02 15:04:05")
	prefix := fmt.Sprintf("[NanoHatOLED] %s ", timeStr)
	l.base.SetPrefix(prefix)
	l.base.Fatalf(format, v...)
}

// initLogger initializes custom logger with local timezone support
func initLogger() {
	initLocalLocation()

	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Printf("Log file error: %v\n", err)
		os.Exit(1)
	}

	baseLogger := log.New(logFile, "", 0)
	logger = &LocalTimeLogger{base: baseLogger}
}

// daemonize converts process to background daemon
func daemonize() error {
	pid, err := syscall.ForkExec("/proc/self/exe", append(os.Args, "--daemon"), &syscall.ProcAttr{
		Env:   os.Environ(),
		Files: []uintptr{os.Stdin.Fd(), os.Stdout.Fd(), os.Stderr.Fd()},
	})
	if err != nil {
		return fmt.Errorf("fork failed: %v", err)
	}
	if pid > 0 {
		os.Exit(0)
	}

	_, err = syscall.Setsid()
	if err != nil {
		return fmt.Errorf("setsid failed: %v", err)
	}

	syscall.Umask(0)
	if err := os.Chdir("/"); err != nil {
		return fmt.Errorf("chdir failed: %v", err)
	}

	nullFd, err := os.OpenFile("/dev/null", os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("open /dev/null failed: %v", err)
	}
	nullFdInt := int(nullFd.Fd())
	unix.Dup2(nullFdInt, 0)
	unix.Dup2(nullFdInt, 1)
	unix.Dup2(nullFdInt, 2)

	return nil
}

// checkSingleInstance ensures only one process runs
func checkSingleInstance() error {
	pidFile, err := os.OpenFile(pidFilePath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("pid file open failed: %v", err)
	}
	defer pidFile.Close()

	if err := unix.Flock(int(pidFile.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if err == unix.EAGAIN || err == unix.EACCES {
			return fmt.Errorf("instance already running")
		}
		return fmt.Errorf("flock failed: %v", err)
	}

	pid := strconv.Itoa(os.Getpid())
	if err := ioutil.WriteFile(pidFilePath, []byte(pid), 0644); err != nil {
		return fmt.Errorf("write pid failed: %v", err)
	}

	return nil
}

// getIP returns eth0 IPv4 address
func getIP() string {
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			if iface.Name == "eth0" && (iface.Flags&net.FlagUp) != 0 {
				addrs, err := iface.Addrs()
				if err != nil {
					continue
				}
				for _, addr := range addrs {
					if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
						return ipNet.IP.String()
					}
				}
			}
		}
	}

	if conn, err := net.Dial("udp", "10.255.255.255:1"); err == nil {
		defer conn.Close()
		return conn.LocalAddr().(*net.UDPAddr).IP.String()
	}

	return "127.0.0.1"
}

// getCPULoad returns 1-minute CPU load average
func getCPULoad() string {
	file, err := os.Open("/proc/loadavg")
	if err != nil {
		logger.Printf("Read loadavg failed: %v", err)
		return "CPU Load: N/A"
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		logger.Printf("Read loadavg content failed: %v", err)
		return "CPU Load: N/A"
	}

	fields := strings.Fields(string(content))
	if len(fields) > 0 {
		load, _ := strconv.ParseFloat(fields[0], 64)
		return fmt.Sprintf("CPU Load: %.2f", load)
	}
	return "CPU Load: N/A"
}

// getMemUsage returns memory usage (used/total MB + percentage)
func getMemUsage() string {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		logger.Printf("Open meminfo failed: %v", err)
		return "Mem: N/A"
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var (
		memTotalKB     int64 = -1
		memFreeKB      int64 = -1
		buffersKB      int64 = -1
		cachedKB       int64 = -1
		sReclaimableKB int64 = -1
	)

	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		fieldName := strings.TrimSpace(parts[0])
		numStr := strings.TrimSpace(strings.ReplaceAll(parts[1], "kB", ""))
		numStr = strings.Join(strings.Fields(numStr), "")

		num, err := strconv.ParseInt(numStr, 10, 64)
		if err != nil {
			logger.Printf("Parse field %s failed: %v", fieldName, err)
			continue
		}

		switch fieldName {
		case "MemTotal":
			memTotalKB = num
		case "MemFree":
			memFreeKB = num
		case "Buffers":
			buffersKB = num
		case "Cached":
			cachedKB = num
		case "SReclaimable":
			sReclaimableKB = num
		}
	}

	if err := scanner.Err(); err != nil {
		logger.Printf("Scan meminfo failed: %v", err)
		return "Mem: N/A"
	}

	if memTotalKB == -1 || memFreeKB == -1 || buffersKB == -1 || cachedKB == -1 || sReclaimableKB == -1 {
		logger.Printf("Missing meminfo fields: MemTotal=%d, MemFree=%d, Buffers=%d, Cached=%d, SReclaimable=%d",
			memTotalKB, memFreeKB, buffersKB, cachedKB, sReclaimableKB)
		return "Mem: N/A"
	}

	buffCacheKB := buffersKB + cachedKB + sReclaimableKB
	usedKB := memTotalKB - memFreeKB - buffCacheKB

	if usedKB < 0 {
		usedKB = 0
	}

	usedMB := int(math.Round(float64(usedKB) / 1024))
	totalMB := int(math.Round(float64(memTotalKB) / 1024))
	percent := float64(usedKB) / float64(memTotalKB) * 100

	return fmt.Sprintf("Mem: %d/%dMB %.1f%%", usedMB, totalMB, percent)
}

// getDiskUsage returns root filesystem usage
func getDiskUsage() string {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		logger.Printf("Statfs failed: %v", err)
		return "Disk: N/A"
	}

	totalBytes := uint64(stat.Blocks) * uint64(stat.Bsize)
	freeBytes := uint64(stat.Bfree) * uint64(stat.Bsize)
	usedBytes := totalBytes - freeBytes

	var pct string
	if totalBytes > 0 {
		usagePercent := float64(usedBytes) / float64(totalBytes) * 100
		pct = fmt.Sprintf("%.0f%%", usagePercent)
	} else {
		pct = "0%"
	}

	const gbThreshold = 1024 * 1024 * 1024
	var used, total float64
	var unit string

	if totalBytes >= gbThreshold {
		used = float64(usedBytes) / gbThreshold
		total = float64(totalBytes) / gbThreshold
		unit = "GB"
		return fmt.Sprintf("Disk: %.1f/%.1f%s %s", used, total, unit, pct)
	} else {
		used = float64(usedBytes) / (1024 * 1024)
		total = float64(totalBytes) / (1024 * 1024)
		unit = "MB"
		return fmt.Sprintf("Disk: %.0f/%.0f%s %s", used, total, unit, pct)
	}
}

// getCPUTemp returns CPU temperature (°C)
func getCPUTemp() string {
	tempData, err := ioutil.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err != nil {
		logger.Printf("Read temp failed: %v", err)
		return "CPU TEMP: N/A°C"
	}
	temp, _ := strconv.Atoi(strings.TrimSpace(string(tempData)))
	if temp > 1000 {
		temp /= 1000
	}
	return fmt.Sprintf("CPU TEMP: %d°C", temp)
}

// getYearProgressText returns year progress bar + percentage
func getYearProgressText() string {
	now := time.Now().In(localLoc)
	start := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, localLoc).Unix()
	end := time.Date(now.Year()+1, 1, 1, 0, 0, 0, 0, localLoc).Unix()
	percent := float64(now.Unix()-start) / float64(end-start) * 100

	barLen := int(percent / 10)
	if barLen < 0 {
		barLen = 0
	} else if barLen > 10 {
		barLen = 10
	}
	bar := strings.Repeat("\u2593", barLen) + strings.Repeat("\u2591", 10-barLen)
	return fmt.Sprintf("%s%.1f%%", bar, percent)
}

// clearTimeArea clears time display region on OLED
func clearTimeArea() {
	oled.Rect(timeX-1, timeY+2, timeX+timeWidth+10, timeY+timeHeight, false)
	oled.SetFontSize(24)
	oled.SetBold(true)
	oled.Text(timeX, timeY, "               ", false)
}

// drawTimePageStatic draws static elements of time page
func drawTimePageStatic() {
	oled.Clear()
	oled.New(0)

	oled.SetFontSize(14)
	oled.SetBold(false)
	oled.Text(2, 2, time.Now().In(localLoc).Format("Mon _2 Jan 2006"), true)
	oled.Text(2, 20, getYearProgressText(), true)

	oled.SetFontSize(24)
	oled.SetBold(true)
	currentTime := time.Now().In(localLoc).Format("15:04:05")
	oled.Text(timeX, timeY, currentTime, true)

	oled.Send()
	lastTimeStr = currentTime
	staticDrawn = true
}

// updateTimeOnly refreshes time value (second-level update)
func updateTimeOnly() {
	currentTime := time.Now().In(localLoc).Format("15:04:05")
	if currentTime == lastTimeStr {
		return
	}

	clearTimeArea()
	oled.Text(timeX, timeY, currentTime, true)
	oled.Send()

	lastTimeStr = currentTime
}

// drawNonTimePage draws system info/shutdown pages
func drawNonTimePage() {
	oled.Clear()
	oled.New(0)

	switch pageIndex {
	case 1:
		oled.SetFontSize(10)
		oled.SetBold(false)
		oled.Text(2, 0, "IP: "+getIP(), true)
		oled.Text(2, 12, getCPULoad(), true)
		oled.Text(2, 24, getMemUsage(), true)
		oled.Text(2, 36, getDiskUsage(), true)
		oled.Text(2, 48, getCPUTemp(), true)

	case 3:
		oled.SetFontSize(14)
		oled.SetBold(true)
		oled.Text(2, 2, "Shutdown?", true)

		oled.SetFontSize(11)
		oled.SetBold(false)
		if shutdownSelect == 0 {
			oled.Rect(2, 20, displayWidth-4, 36, true)
			oled.Text(4, 22, "Yes", false)
			oled.Rect(2, 38, displayWidth-4, 54, false)
			oled.Text(4, 40, "No", true)
		} else {
			oled.Rect(2, 20, displayWidth-4, 36, false)
			oled.Text(4, 22, "Yes", true)
			oled.Rect(2, 38, displayWidth-4, 54, true)
			oled.Text(4, 40, "No", false)
		}
	}

	oled.Send()
	lastPageIndex = pageIndex
	lastShutdownSel = shutdownSelect
}

// drawPage handles main OLED page rendering logic
func drawPage() {
	pageMutex.Lock()
	defer pageMutex.Unlock()

	if drawing || shutdownFlag {
		return
	}

	if pageSleepCount <= 0 {
		if pageSleepCount == 0 {
			oled.Clear()
			oled.Send()
			staticDrawn = false
			lastPageIndex = -1
			pageSleepCount = -1
		}
		return
	}
	pageSleepCount--

	drawing = true
	defer func() { drawing = false }()

	switch pageIndex {
	case 0:
		if !staticDrawn || lastPageIndex != 0 {
			drawTimePageStatic()
			lastPageIndex = 0
		} else {
			updateTimeOnly()
		}

	case 1, 3:
		if pageIndex != lastPageIndex || (pageIndex == 3 && shutdownSelect != lastShutdownSel) {
			drawNonTimePage()
		}
	}
}

// resetSleepCount resets page sleep counter
func resetSleepCount() {
	pageMutex.Lock()
	pageSleepCount = pageSleep
	staticDrawn = false
	pageMutex.Unlock()
}

// handleK1 processes K1 button press events
func handleK1() {
	logger.Println("K1 pressed")
	resetSleepCount()

	pageMutex.Lock()
	defer pageMutex.Unlock()

	if pageIndex == 3 {
		shutdownSelect = (shutdownSelect + 1) % 2
	} else {
		pageIndex = 0
	}
}

// handleK2 processes K2 button press events
func handleK2() {
	logger.Println("K2 pressed")
	resetSleepCount()

	pageMutex.Lock()
	defer pageMutex.Unlock()

	if pageIndex == 3 {
		if shutdownSelect == 0 {
			shutdownFlag = true
		} else {
			pageIndex = 0
		}
		return
	}
	pageIndex = 1
}

// handleK3 processes K3 button press events
func handleK3() {
	logger.Println("K3 pressed")
	resetSleepCount()

	pageMutex.Lock()
	defer pageMutex.Unlock()

	if pageIndex == 3 {
		pageIndex = 0
	} else {
		pageIndex = 3
		shutdownSelect = 1
	}
}

// watchButtons monitors button events in goroutines
func watchButtons() {
	watchBtn := func(btnIdx int, handler func()) {
		for {
			if oled.Btn[btnIdx].WaitForEdge(-1) {
				time.Sleep(150 * time.Millisecond)
				handler()
				drawPage()
			} else {
				time.Sleep(100 * time.Millisecond)
			}
		}
	}

	go watchBtn(btnK1, handleK1)
	go watchBtn(btnK2, handleK2)
	go watchBtn(btnK3, handleK3)
}

// doShutdown executes system shutdown procedure
func doShutdown() {
	pageMutex.Lock()
	defer pageMutex.Unlock()

	logger.Println("Executing shutdown...")
	oled.Clear()
	oled.New(0)
	oled.SetFontSize(14)
	oled.SetBold(true)
	oled.Text(2, 2, "Shutting down", true)
	oled.SetFontSize(11)
	oled.SetBold(false)
	oled.Text(2, 20, "Please wait...", true)
	oled.Send()

	time.Sleep(2 * time.Second)

	oled.Clear()
	oled.Send()
	time.Sleep(300 * time.Millisecond)

	os.Remove(pidFilePath)

	if err := syscall.Exec("/sbin/poweroff", []string{"poweroff"}, os.Environ()); err != nil {
		logger.Printf("Poweroff failed: %v", err)
	}
	os.Exit(0)
}

// processExists checks if process with given PID is running
func processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	return err == syscall.EPERM
}

// main is program entry point
func main() {
	shutdownSelect = 1

	if len(os.Args) > 1 && os.Args[1] == "-stop" {
		fmt.Println("Clearing OLED screen...")
		stopOled, err := nanohatoled.Open()
		if err != nil {
			fmt.Printf("Failed to open OLED for clear: %v\n", err)
		} else {
			defer stopOled.Close()
			stopOled.Clear()
			stopOled.Send()
			time.Sleep(300 * time.Millisecond)
			fmt.Println("Screen cleared successfully")
		}

		if _, err := os.Stat(pidFilePath); os.IsNotExist(err) {
			fmt.Println("Daemon is not running (PID file not found)")
			os.Exit(0)
		}

		pidData, err := ioutil.ReadFile(pidFilePath)
		if err != nil {
			fmt.Printf("Failed to read PID file: %v\n", err)
			os.Remove(pidFilePath)
			os.Exit(1)
		}

		pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
		if err != nil {
			fmt.Printf("Invalid PID in file: %v\n", err)
			os.Remove(pidFilePath)
			os.Exit(1)
		}

		if !processExists(pid) {
			fmt.Printf("Daemon process (PID: %d) does not exist\n", pid)
			os.Remove(pidFilePath)
			os.Exit(0)
		}

		if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
			fmt.Printf("Failed to send SIGTERM to PID %d: %v\n", pid)
			if processExists(pid) {
				fmt.Printf("Trying to force kill PID %d...\n", pid)
				if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
					fmt.Printf("Force kill failed: %v\n", err)
					os.Exit(1)
				}
			}
		}

		os.Remove(pidFilePath)
		fmt.Printf("Daemon (PID: %d) stopped successfully\n", pid)
		os.Exit(0)
	}

	initLogger()
	if err := checkSingleInstance(); err != nil {
		logger.Fatalf("Instance error: %v", err)
	}

	if len(os.Args) == 1 {
		if err := daemonize(); err != nil {
			logger.Fatalf("Daemon error: %v", err)
		}
	}

	var err error
	oled, err = nanohatoled.Open()
	if err != nil {
		logger.Fatalf("OLED init failed: %v", err)
	}
	defer oled.Close()

	pageMutex.Lock()
	logger.Println("Display logo...")
	oled.Clear()
	oled.New(0)
	if _, err := os.Stat(logoPath); os.IsNotExist(err) {
		logger.Printf("Logo not found: %s", logoPath)
		oled.Text(2, 20, "No Logo", true)
	} else if err := oled.Image(logoPath); err != nil {
		logger.Printf("Logo load failed: %v", err)
		oled.Text(2, 20, "Logo Err", true)
	}
	oled.Send()
	pageMutex.Unlock()
	time.Sleep(2 * time.Second)

	pageMutex.Lock()
	pageIndex = 0
	pageSleepCount = pageSleep
	drawing = false
	shutdownFlag = false
	lastPageIndex = -1
	lastTimeStr = ""
	lastShutdownSel = -1
	staticDrawn = false
	pageMutex.Unlock()

	watchButtons()

	logger.Println("Main loop started")
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		pageMutex.Lock()
		isShutdown := shutdownFlag
		pageMutex.Unlock()

		if isShutdown {
			doShutdown()
			return
		}

		drawPage()
	}
}
