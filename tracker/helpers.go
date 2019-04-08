package tracker

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/fearful-symmetry/garlic"
	"github.com/mitchellh/go-ps"
)

func getNameByPid(pid uint32) string {
	pidInfo, err := ps.FindProcess(int(pid))
	if err != nil || pidInfo == nil {
		return ""
	}
	return pidInfo.Executable()
}

func getArgsByPid(pid uint32) string {
	cmdline := fmt.Sprintf("/proc/%v/cmdline", pid)
	reg, _ := regexp.Compile("[^a-zA-Z0-9]+")
	if Exists(cmdline) {
		d, err := ioutil.ReadFile(cmdline)
		if err == nil {
			filtered := reg.ReplaceAllString(string(d), "")
			return filtered
		}
	}
	return ""
}

func printInfo(event garlic.ProcEvent) {
	if event.WhatString == "Exit" {
		return
	}
	cmdline := fmt.Sprintf("/proc/%v/cmdline", event.EventData.Pid())
	args := ""
	if Exists(cmdline) {
		d, err := ioutil.ReadFile(cmdline)
		if err == nil {
			args = string(d)
		}
	}
	if t, ok := event.EventData.(garlic.Fork); ok {
		fmt.Printf("Event: %s; PID: %v; ParentPID: %v; TGID: %v; Args: %s\n\n", event.WhatString, event.EventData.Pid(), uint32(t.ParentPid), t.Tgid(), args)
		printInfoByPID("fork", t.ParentPid)
		return
	}
	if t, ok := event.EventData.(garlic.Comm); ok {
		fmt.Printf("Event: %s; PID: %v; Command: %v; Args: %s\n\n", event.WhatString, event.EventData.Pid(), t.Comm, args)
		return
	}
	if t, ok := event.EventData.(garlic.Exec); ok {
		fmt.Printf("Event: %s; PID: %v; TGID: %v; Args: %s\n\n", event.WhatString, event.EventData.Pid(), t.ProcessTgid, args)
		return
	}

	fmt.Printf("Event: %s; PID: %v; Args: %s\n\n", event.WhatString, event.EventData.Pid(), args)
}

func printInfoByPID(action string, pid uint32) {
	if pid == uint32(1) {
		return
	}
	statFile := fmt.Sprintf("/proc/%v/status", pid)
	stat := ""
	if Exists(statFile) {
		s, err := ioutil.ReadFile(statFile)
		if err == nil {
			stat = string(s)
		}
		if strings.Contains(stat, "Tgid:") && strings.Contains(stat, "PPid:	1") {
			//parse out the Tgid and get pid info by that pid
			parts := strings.Split(stat, "\n")
			for _, v := range parts {
				if strings.Contains(v, "Tgid:") {
					//split 'Tgid: int'
					tgidparts := strings.Split(v, ":")
					tgid := strings.TrimSpace(tgidparts[1])
					fmt.Printf("FOUND A TGID %v\n\n\n\n", tgid)
					if s, err := strconv.Atoi(tgid); err == nil {
						if uint32(s) == pid {
							return
						}
						printInfoByPID("TGIDSPAWN", uint32(s))
					}
				}
			}
		}
	}

	tp := TrackedProcess{}

	pidInfo, err := ps.FindProcess(int(pid))
	if err != nil {
		tp.Name = "unknown"
	}
	if pidInfo != nil {
		tp.Name = pidInfo.Executable()
	}

	//check if pid cmdline info exists
	cmdline := fmt.Sprintf("/proc/%v/cmdline", pid)
	if Exists(cmdline) {
		d, err := ioutil.ReadFile(cmdline)
		if err == nil {
			tp.Args = string(d)
		}
	}

	fmt.Printf("PID: %v - Process %s %s with args %s; stats%s\n", pid, tp.Name, action, tp.Args, stat)
}

// Exists reports whether the named file or directory exists.
//https://stackoverflow.com/a/12527546
func Exists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}
