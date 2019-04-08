# Docker Run Time Inspector

Howdy!

This wasn't as easy as I originally planned. I created a simple runtime detector that worked fine outside of a container. As soon as I threw a container at it though, it failed to give any useful information other than the entire build process.

The reason this occurred, as far as I could research with my limited time, was that the actual "docker build" commands are actually spawned via container/dockerd, and are *not* children of the build process. So, I couldn't just watch the build process for fork/execs. 

Instead, I did something very hacky. It works, and I know there's room for improvement, but i'm not completely satisfied with it. Primarily, I just watch for new processes that get forked by containerd/dockerd (it seemed through testing that both were important). Then, I begin watching those new processes for forks, until I find an exec in the chain who's parent belongs to runcinit. Note that *runcinit* may just by my init system and not yours, so this may not work on your system.

There is lots of other tracking being done as well, so this could also potentially spit out the timings for all the processes forked by the initial build process. However most of this data is how long docker itself takes to perform certain actions preparing an environment for an actual Dockerfile command, and not the command itself.

## Running
Just `sudo go run main.go`.

You can toggle the commented out runlines in main.go to change from the small dockerfile to the one you provided.

## Dependendencies/Environment

Hey, you didn't give me any constraints, so here's what is required for this to work:
* You have to be using Linux. I doubt this works on a Mac, or docker for Mac (although it might)
* The kernel must be built with CONFIG_PROC_EVENTS enabled.
* The init system must be called "runcinit".

All of the above works in Arch Linux with kernel 4.20+

## Cons/Areas of Improvement

* Although I didn't test this, I think that if other containers are being built at the same time, this will pick up those commands, *even if they aren't part of the originally tracked build command*. I feel about 50/50 that this can be solved, however I just didn't have enough time to go down that rabbit hole and figure it out.
* Also, it requires the CONFIG_PROC_EVENTS kernel option to be enabled. This means it may not work in the Mac docker vm.
* It doesn't catch and track "&&" commands, although this could be done.
* Lastly, I don't watch the process tree for docker-in-docker commands, or really any commands that get spawned by the original "docker build" commands, however this definitely could be done fairly easily. 
* It relies on a library that needs work. Specifically, sometimes when starting it up, it spits out data error data and the process has to be restarted. example:
```bash
2019/04/08 10:54:36 error initiliazing garlic: Packet not a valid ACK: [{Header:{Length:76 Type:done Flags:0 Sequence:87065 PID:0} Data:[1 0 0 0 1 0 0 0 25 84 1 0 0 0 0 0 40 0 0 0 1 0 0 0 1 0 0 0 39 80 100 55 164 10 0 0 55 96 0 0 6 96 0 0 73 96 0 0 68 96 0 0 0 0 0 0 0 0 0 0]}]
exit status 1
```

## Advantages

There is *very little* overhead to this approach, as the entire tracking process is completed by receiving events from the kernel, and the only polling done is triggered by events where new information is known to exist. 

## Sources

* https://bewareofgeek.livejournal.com/2945.html
* https://github.com/fearful-symmetry/garlic
* https://medium.com/@mdlayher/linux-netlink-and-go-part-2-generic-netlink-833bce516400

## Tests

The tests are trash. I abandoned them after I realized the entire workflow had to be reworked. This entire codebase is rather hacky, so it needs to be refactored altogether, let alone making it testable. 

## Sample outputs:
Dockerfile.small
```bash
2019/04/08 10:54:38 Garlic connection created.
2019/04/08 10:54:38 Watching process docker(24722) dockerbuildfDockerfilesmallnocache

2019/04/08 10:54:38 Exec of binshcapkupdate(24765) detected and attaching to runc:[1:CHILD]
2019/04/08 10:54:39 EXIT: 24765; Time: 616.178937ms; Args: binshcapkupdate

2019/04/08 10:54:40 Exec of binshcapkaddnano(24857) detected and attaching to runc:[2:INIT]
2019/04/08 10:54:41 EXIT: 24857; Time: 606.408402ms; Args: binshcapkaddnano

2019/04/08 10:54:42 ==============Results============



2019/04/08 10:54:42 24765 -> Exec (binshcapkupdate); Total Time: 616.178937ms
2019/04/08 10:54:42 24857 -> Exec (binshcapkaddnano); Total Time: 606.408402ms
2019/04/08 10:54:42 Full runtime: 3.827511245s
```

Dockerfile
```bash
2019/04/08 10:38:58 Garlic connection created.
2019/04/08 10:38:58 Watching process docker(6266) dockerbuildnocache

Exec of binshcaptgetupdate(6307) detected and attaching to runc:[2:INIT]
2019/04/08 10:39:03 EXIT: 6307; Time: 4.392874872s; Args: binshcaptgetupdate

Exec of binshcaptgetinstallywgetcurlbuildessentialgitgitcorezlib1gdevlibssldevlibreadlinedevlibyamldevlibsqlite3devsqlite3libxml2devlibxslt1dev(6613) detected and attaching to runc:[1:CHILD]
2019/04/08 10:39:56 EXIT: 6613; Time: 52.387636438s; Args: binshcaptgetinstallywgetcurlbuildessentialgitgitcorezlib1gdevlibssldevlibreadlinedevlibyamldevlibsqlite3devsqlite3libxml2devlibxslt1dev

Exec of binshcaptgetupdate(11287) detected and attaching to runc:[2:INIT]
2019/04/08 10:40:02 EXIT: 11287; Time: 1.438241206s; Args: binshcaptgetupdate

Exec of binshccdtmpwgetOrubyinstall061targzhttpsgithubcompostmodernrubyinstallarchivev061targztarxzvfrubyinstall061targzcdrubyinstall061makeinstall(11663) detected and attaching to runc:[1:CHILD]
2019/04/08 10:40:04 EXIT: 11663; Time: 577.501838ms; Args: binshccdtmpwgetOrubyinstall061targzhttpsgithubcompostmodernrubyinstallarchivev061targztarxzvfrubyinstall061targzcdrubyinstall061makeinstall

Exec of binshcrubyinstallruby240(11869) detected and attaching to runc:[2:INIT]
2019/04/08 10:44:52 EXIT: 11869; Time: 4m47.343340236s; Args: binshcrubyinstallruby240

Exec of (23490) detected and attaching to runc:[1:CHILD]
2019/04/08 10:45:02 EXIT: 23490; Time: 13.38166ms; Args: 

Exec of binshcgeminstallbundler(23573) detected and attaching to runc:[1:CHILD]
2019/04/08 10:45:05 EXIT: 23573; Time: 1.009009635s; Args: binshcgeminstallbundler

2019/04/08 10:45:07 ==============Results============



2019/04/08 10:45:07 6307 -> Exec (binshcaptgetupdate); Total Time: 4.392874872s
2019/04/08 10:45:07 6613 -> Exec (binshcaptgetinstallywgetcurlbuildessentialgitgitcorezlib1gdevlibssldevlibreadlinedevlibyamldevlibsqlite3devsqlite3libxml2devlibxslt1dev); Total Time: 52.387636438s
2019/04/08 10:45:07 11287 -> Exec (binshcaptgetupdate); Total Time: 1.438241206s
2019/04/08 10:45:07 11663 -> Exec (binshccdtmpwgetOrubyinstall061targzhttpsgithubcompostmodernrubyinstallarchivev061targztarxzvfrubyinstall061targzcdrubyinstall061makeinstall); Total Time: 577.501838ms
2019/04/08 10:45:07 11869 -> Exec (binshcrubyinstallruby240); Total Time: 4m47.343340236s
2019/04/08 10:45:07 23490 -> Exec (); Total Time: 13.38166ms
2019/04/08 10:45:07 23573 -> Exec (binshcgeminstallbundler); Total Time: 1.009009635s
2019/04/08 10:45:07 Full runtime: 6m8.799286044s
```