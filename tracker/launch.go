package tracker

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/mitchellh/go-ps"

	"github.com/fearful-symmetry/garlic"
)

type TrackedProcess struct {
	Children []*TrackedProcess
	Parent   *TrackedProcess
	PID      uint32
	Start    time.Time
	End      time.Time
	Done     bool
	Name     string
	Args     string
	Exec     []*Exec
}

type Exec struct {
	PID    uint32
	Start  time.Time
	End    time.Time
	Done   bool
	Args   string
	Parent *TrackedProcess
}

//to do this appropriately, we need to:
//1. buffer all incoming PCN calls.
//2. Start the process, and get it's PID
//3. Replay the buffer, to look for events
//4. Capture events as normal

//Launch will launch a process with the given arguments,
//and tracks process completion times.
func Launch(g *garlic.CnConn, name string, arg ...string) {
	ctx, _ := context.WithCancel(context.Background())

	events := make(chan []garlic.ProcEvent, 50)
	parentProcessChan := make(chan *TrackedProcess, 1)
	watchingCancel := make(chan bool, 1)
	loopCancel := make(chan bool, 1)
	go startWatching(g, events, watchingCancel)

	// launch the process
	cmd := exec.CommandContext(ctx, name, arg...)
	cmd.Stdout = nil
	cmd.Stderr = os.Stderr
	// get the pid
	err := cmd.Start()
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	tp := &TrackedProcess{
		Start: time.Now(),
		PID:   uint32(cmd.Process.Pid),
		Args:  getArgsByPid(uint32(cmd.Process.Pid)),
	}
	pidInfo, err := ps.FindProcess(cmd.Process.Pid)
	if err != nil {
		tp.Name = name
	} else {
		tp.Name = pidInfo.Executable()
	}

	//send the process to be tracked
	parentProcessChan <- tp

	//TODO: set a timeout and cancel if we're over the limit
	eventLoop(events, parentProcessChan, loopCancel)

	// once everything is done, or a timeout occurs,
	// print out the call stack/times
	print(tp)
}

func startWatching(gC *garlic.CnConn, events chan []garlic.ProcEvent, cancel chan bool) {
	for {
		select {
		case <-cancel:
			//just close the loop
			return
		default:
			data, err := gC.ReadPCN()
			if err != nil {
				log.Printf("Error reading from PCN: %s\n", err)
				continue
			}
			events <- data
		}
	}
}

func eventLoop(eventsChan chan []garlic.ProcEvent, parentProcessChan chan *TrackedProcess, cancelChan chan bool) {
	var err error
	var finished bool
	var parentProcess *TrackedProcess
	for {
		select {
		case <-cancelChan:
			//TODO: print out the current results? this will be incomplete,
			//and the printProcessTree expects an end time, so this is work to be done.
			return
		case parentProcess = <-parentProcessChan:
			//add this TrackedProcess to be watched by a sentry
			log.Printf("Watching process %s(%v) %s\n\n", parentProcess.Name, parentProcess.PID, parentProcess.Args)
			continue
		case newEvents := <-eventsChan:
			if parentProcess == nil {
				//TODO: we don't have a parent process yet, so cache the event for now. This is an
				//edge case that needs to be covered.
				continue
			}
			//handle newEvents
			finished, err = handleEvents(newEvents, parentProcess)
			if err != nil {
				//TODO: panics be bad
				log.Panicf("Unable to handleEvents: %v\n", err)
			}
			if finished {
				return
			}
		}
	}
}

func handleEvents(data []garlic.ProcEvent, parent *TrackedProcess) (bool, error) {
	for _, d := range data {
		// printInfo(d)
		// fmt.Printf("%#v Args: %s\n\n", data, getArgsByPid(d.EventData.Pid()))
		// fmt.Println(fmt.Sprintf("New Data: %v, Type: %v", d.EventData.Pid(), d.WhatString))

		//check if this is a fork
		if t, ok := d.EventData.(garlic.Fork); ok {
			handleFork(parent, t)
			continue
		}

		//check if this is an exec
		if t, ok := d.EventData.(garlic.Exec); ok {
			handleExec(parent, t)
			continue
		}

		if t, ok := d.EventData.(garlic.Exit); ok {
			handleExit(parent, t)
			continue
		}

		//check if the pid of d is in our map
		process := searchTrackedProcesses(parent, d.EventData.Pid())
		if process == nil {
			//nope, exit early
			continue
		}
	}
	//
	return parent.Done == true, nil
}

func handleFork(parent *TrackedProcess, e garlic.Fork) {
	parentName := getNameByPid(e.ParentPid)
	name := getNameByPid(e.Pid())
	//if this is a fork of containerd/dockerd, begin watching it
	if parentName == "dockerd" || parentName == "containerd" {
		tp := &TrackedProcess{
			Parent: parent, //this isn't actually true, it's spawned by run-init
			PID:    e.Pid(),
			Start:  time.Now(),
			Name:   getNameByPid(e.Pid()),
			Args:   getArgsByPid(e.Pid()),
		}
		// fmt.Printf("Fork of `dockerd/containerd`(%v) %s detected\n", tp.PID, tp.Args)
		parent.Children = append(parent.Children, tp)
		return
	}

	//determine if this is a duplicate

	//otherwise check if this is a fork of a process we are already watching
	parentPID := uint32(e.ParentPid)
	if process := searchTrackedProcesses(parent, parentPID); process != nil {
		tp := &TrackedProcess{
			PID:   e.Pid(),
			Start: time.Now(),
			Name:  name,
			Args:  getArgsByPid(e.Pid()),
		}
		process.Children = append(process.Children, tp)
		// if strings.Contains(tp.Name, "run") {
		// 	fmt.Printf("Found a fork of PID: %v; New PID: %v With Args: %s\n", parentPID, process.PID, tp.Args)
		// }
	}
}

func handleExec(parent *TrackedProcess, e garlic.Exec) {
	exec := &Exec{
		PID:   e.Pid(),
		Args:  getArgsByPid(e.Pid()),
		Start: time.Now(),
		Done:  false,
	}
	// fmt.Printf("Exec of %s(%v) detected\n", exec.Args, exec.PID)
	if len(parent.Children) == 0 {
		// fmt.Println("No siblings detected, so not adding exec.")
		return
	}
	previousExec := searchExecutions(parent, e.ProcessPid)
	if previousExec != nil {
		//already added this exec, don't do it again?
		// log.Println("Detected an execution that was already recorded, skipping.")
		return
	}
	//Don't need to execs of runcinit
	if exec.Args == "runcinit" {
		return
	}
	tp := searchTrackedProcesses(parent, e.ProcessPid)
	if tp == nil {
		// fmt.Printf("Unable to find a tracked process chain for this exec. %s\n", exec.Args)
		return
	}
	//detect if this was spawned by a process called runcinit
	if tp.Args != "runcinit" {
		return
	}
	exec.Parent = tp
	if tp.Exec == nil {
		tp.Exec = make([]*Exec, 0)
	}
	fmt.Printf("Exec of %s(%v) detected and attaching to %s\n", exec.Args, exec.PID, tp.Name)
	tp.Exec = append(tp.Exec, exec)
}

func handleExit(parent *TrackedProcess, e garlic.Exit) {
	tp := searchTrackedProcesses(parent, e.Pid())
	te := searchExecutions(parent, e.Pid())
	if tp == nil && te == nil {
		return
	}
	if tp != nil {
		//TODO: could print this information out as well, but it's mostly timers for docker libs
		tp.End = time.Now()
		tp.Done = true
	}
	if te != nil {
		te.End = time.Now()
		te.Done = true
		log.Printf("EXIT: %v; Time: %v; Args: %s\n\n", e.Pid(), te.End.Sub(te.Start), te.Args)
	}
}

func searchExecutions(process *TrackedProcess, pid uint32) *Exec {
	for _, e := range process.Exec {
		if e.PID == pid {
			return e
		}
	}
	if len(process.Children) == 0 {
		return nil
	}
	for _, c := range process.Children {
		found := searchExecutions(c, pid)
		if found != nil {
			return found
		}
	}
	return nil
}

//searchTrackedProcesses searches all of the process trees we know about. Currently I can't find a way to track
//the containerd forks that eventually execute commands in the dockerfile from the `docker build` command.
//Instead I just capture all containderd forks that lead to spawns, and they are each an individual tree that gets
//searched for.
func searchTrackedProcesses(process *TrackedProcess, pid uint32) *TrackedProcess {
	//this is the process we are looking for
	if process.PID == pid {
		return process
	}
	if len(process.Children) == 0 {
		//this process has no child processes
		return nil
	}
	for _, v := range process.Children {
		ret := searchTrackedProcesses(v, pid)
		if ret != nil {
			return ret
		}
	}
	return nil
}

func print(process *TrackedProcess) {
	log.Printf("==============Results============\n\n\n\n")
	printProcessTree(process, "")
	log.Printf("Full runtime: %v\n", process.End.Sub(process.Start))
}

//printProcessTree creates the `ProgName (pid) Total Time: <time> -> ProgName etc...` information
func printProcessTree(process *TrackedProcess, output string) {
	if process == nil {
		return
	}
	for _, e := range process.Exec {
		log.Printf("%v -> Exec (%v); Total Time: %v\n", e.PID, e.Args, e.End.Sub(e.Start))
	}
	for _, p := range process.Children {
		printProcessTree(p, output)
	}
}
