//go:build windows

package collectors

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	gpsmem "github.com/shirou/gopsutil/v3/mem"
	gpsproc "github.com/shirou/gopsutil/v3/process"
)

type ProcessInfo struct {
	PID           int      `json:"pid"`
	PPID          int      `json:"ppid"`
	Name          string   `json:"name"`
	Command       string   `json:"command"`
	CommandLine   []string `json:"commandLine,omitempty"`
	State         string   `json:"state"`
	User          string   `json:"user"`
	UID           int      `json:"uid"`
	GID           int      `json:"gid"`
	CPUPercent    float64  `json:"cpuPercent"`
	MemoryPercent float64  `json:"memoryPercent"`
	MemoryBytes   uint64   `json:"memoryBytes"`
	Threads       int      `json:"threads"`
	Nice          int      `json:"nice"`
	VmSize        uint64   `json:"vmSize,omitempty"`
	VmRss         uint64   `json:"vmRss,omitempty"`
	VmSwap        uint64   `json:"vmSwap,omitempty"`
	IoReadBytes   uint64   `json:"ioReadBytes,omitempty"`
	IoWriteBytes  uint64   `json:"ioWriteBytes,omitempty"`
	Exe           string   `json:"exe,omitempty"`
	Cwd           string   `json:"cwd,omitempty"`
	Uptime        string   `json:"uptime,omitempty"`
	Children      []int    `json:"children,omitempty"`
	Connections   []Socket `json:"connections,omitempty"`
	FDs           []FD     `json:"fds,omitempty"`
}

type FD struct {
	FD     int    `json:"fd"`
	Type   string `json:"type"`
	Target string `json:"target"`
}

type ProcessList struct {
	Processes  []ProcessInfo `json:"processes"`
	TotalCount int           `json:"totalCount"`
}


var (
	prevProcCPU   = map[int32]float64{}
	prevProcCPUMu sync.Mutex

	processListCache    ProcessList
	processListCachedAt time.Time
	processListMu       sync.Mutex
	processListTTL      = 6 * time.Second
)

func GetProcessList() (ProcessList, error) {
	processListMu.Lock()
	if !processListCachedAt.IsZero() && time.Since(processListCachedAt) < processListTTL {
		cached := processListCache
		processListMu.Unlock()
		return cached, nil
	}
	processListMu.Unlock()

	list := ProcessList{Processes: []ProcessInfo{}}

	var totalMemory uint64
	if vm, err := gpsmem.VirtualMemory(); err == nil {
		totalMemory = vm.Total
	}

	pids, err := gpsproc.Pids()
	if err != nil {
		return list, err
	}

	prevProcCPUMu.Lock()
	prevSnapshot := prevProcCPU
	prevProcCPUMu.Unlock()

	type entry struct {
		pi  ProcessInfo
		cur float64
	}

	in := make(chan int32, len(pids))
	out := make(chan entry, len(pids))
	for _, pid := range pids {
		in <- pid
	}
	close(in)

	// LookupAccountSid (gopsutil Username) is the main cost per process; with
	// 300+ PIDs and synchronous calls this loop took >10s. Fan out across
	// workers — each call opens and closes its own handle, so they're safe
	// to run in parallel.
	workers := 32
	if len(pids) < workers {
		workers = len(pids)
	}
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for pid := range in {
				p, err := gpsproc.NewProcess(pid)
				if err != nil {
					continue
				}
				pi := ProcessInfo{PID: int(pid), State: "running"}
				if name, err := p.Name(); err == nil {
					pi.Name = name
				}
				if ppid, err := p.Ppid(); err == nil {
					pi.PPID = int(ppid)
				}
				if user, err := p.Username(); err == nil {
					pi.User = user
				}
				if mem, err := p.MemoryInfo(); err == nil && mem != nil {
					pi.MemoryBytes = mem.RSS
					pi.VmRss = mem.RSS
					pi.VmSize = mem.VMS
					if totalMemory > 0 {
						pi.MemoryPercent = float64(mem.RSS) / float64(totalMemory) * 100
					}
				}
				if threads, err := p.NumThreads(); err == nil {
					pi.Threads = int(threads)
				}
				var cur float64
				if times, err := p.Times(); err == nil {
					cur = times.User + times.System
					if prev, ok := prevSnapshot[pid]; ok && cur >= prev {
						pi.CPUPercent = cur - prev
					}
				}
				out <- entry{pi: pi, cur: cur}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	newPrev := make(map[int32]float64, len(pids))
	for e := range out {
		newPrev[int32(e.pi.PID)] = e.cur
		list.Processes = append(list.Processes, e.pi)
	}

	prevProcCPUMu.Lock()
	prevProcCPU = newPrev
	prevProcCPUMu.Unlock()

	list.TotalCount = len(list.Processes)

	processListMu.Lock()
	processListCache = list
	processListCachedAt = time.Now()
	processListMu.Unlock()

	return list, nil
}

// detailedProcessInfo fills in slower fields (cwd, IO, uptime) via gopsutil
// for a single process where the extra latency is acceptable.
func detailedProcessInfo(p *gpsproc.Process, totalMemory uint64) ProcessInfo {
	pi := ProcessInfo{PID: int(p.Pid), State: "running"}

	if name, err := p.Name(); err == nil {
		pi.Name = name
	}
	if ppid, err := p.Ppid(); err == nil {
		pi.PPID = int(ppid)
	}
	if user, err := p.Username(); err == nil {
		pi.User = user
	}
	if cpu, err := p.CPUPercent(); err == nil {
		pi.CPUPercent = cpu
	}
	if mem, err := p.MemoryInfo(); err == nil && mem != nil {
		pi.MemoryBytes = mem.RSS
		pi.VmRss = mem.RSS
		pi.VmSize = mem.VMS
		pi.VmSwap = mem.Swap
		if totalMemory > 0 {
			pi.MemoryPercent = float64(mem.RSS) / float64(totalMemory) * 100
		}
	}
	if threads, err := p.NumThreads(); err == nil {
		pi.Threads = int(threads)
	}
	if exe, err := p.Exe(); err == nil {
		pi.Exe = exe
	}
	if cwd, err := p.Cwd(); err == nil {
		pi.Cwd = cwd
	}
	if cmd, err := p.Cmdline(); err == nil && cmd != "" {
		pi.Command = cmd
	} else if pi.Exe != "" {
		pi.Command = pi.Exe
	}
	if cmdSlice, err := p.CmdlineSlice(); err == nil && len(cmdSlice) > 0 {
		pi.CommandLine = cmdSlice
	}
	if io, err := p.IOCounters(); err == nil && io != nil {
		pi.IoReadBytes = io.ReadBytes
		pi.IoWriteBytes = io.WriteBytes
	}
	if created, err := p.CreateTime(); err == nil && created > 0 {
		uptime := time.Since(time.Unix(created/1000, 0))
		hours := int(uptime.Hours())
		minutes := int(uptime.Minutes()) % 60
		pi.Uptime = fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return pi
}

func GetProcessDetail(pid int) (*ProcessInfo, error) {
	p, err := gpsproc.NewProcess(int32(pid))
	if err != nil {
		return nil, err
	}

	var totalMemory uint64
	if vm, err := gpsmem.VirtualMemory(); err == nil {
		totalMemory = vm.Total
	}

	pi := detailedProcessInfo(p, totalMemory)

	if children, err := p.Children(); err == nil {
		for _, c := range children {
			pi.Children = append(pi.Children, int(c.Pid))
		}
	}

	return &pi, nil
}

func GetProcessesByUser(username string) ([]ProcessInfo, error) {
	list, err := GetProcessList()
	if err != nil {
		return nil, err
	}

	var result []ProcessInfo
	for _, p := range list.Processes {
		if strings.EqualFold(p.User, username) || strings.EqualFold(plainUser(p.User), username) {
			result = append(result, p)
		}
	}
	return result, nil
}

// plainUser returns just the username portion of "DOMAIN\user".
func plainUser(s string) string {
	if i := strings.LastIndex(s, `\`); i >= 0 {
		return s[i+1:]
	}
	return s
}

// KillProcess terminates a process on Windows.
func KillProcess(pid int, signal syscall.Signal) error {
	if p, err := gpsproc.NewProcess(int32(pid)); err == nil {
		if err := p.Kill(); err == nil {
			return nil
		}
	}
	cmd := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid))
	return cmd.Run()
}

// ReniceProcess changes process priority on Windows.
func ReniceProcess(pid int, priority int) error {
	var priorityClass string
	switch {
	case priority >= 15:
		priorityClass = "64" // Idle
	case priority >= 5:
		priorityClass = "16384" // Below Normal
	case priority >= -5:
		priorityClass = "32" // Normal
	case priority >= -10:
		priorityClass = "32768" // Above Normal
	default:
		priorityClass = "128" // High
	}

	script := `(Get-Process -Id ` + strconv.Itoa(pid) + `).PriorityClass = ` + priorityClass
	_, err := runPowerShell(script)
	return err
}
