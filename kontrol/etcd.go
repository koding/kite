/*
Copyright 2013 CoreOS Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kontrol

import (
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"time"

	"github.com/coreos/etcd/config"
	ehttp "github.com/coreos/etcd/http"
	elog "github.com/coreos/etcd/log"
	"github.com/coreos/etcd/metrics"
	"github.com/coreos/etcd/server"
	"github.com/coreos/etcd/store"
	"github.com/coreos/etcd/third_party/github.com/coreos/raft"
)

// This function is copied and modified from github.com/coreos/etcd/main.go file.
func (k *Kontrol) runEtcd(ready chan bool) {
	// Load configuration.
	var config = config.New()
	config.Load(nil)

	advertiseIP := k.ip
	if advertiseIP == "0.0.0.0" {
		advertiseIP = "127.0.0.1"
	}

	// Load config values from kontrol
	config.Name = k.name
	config.DataDir = k.dataDir

	config.BindAddr = k.ip + ":" + strconv.Itoa(k.port+1)
	config.Addr = "http://" + advertiseIP + ":" + strconv.Itoa(k.port+1)

	config.Peer.BindAddr = k.ip + ":" + strconv.Itoa(k.port+3001)
	config.Peer.Addr = "http://" + advertiseIP + ":" + strconv.Itoa(k.port+3001)

	config.Peers = k.peers

	// Enable options.
	if config.VeryVeryVerbose {
		elog.Verbose = true
		raft.SetLogLevel(raft.Trace)
	} else if config.VeryVerbose {
		elog.Verbose = true
		raft.SetLogLevel(raft.Debug)
	} else if config.Verbose {
		elog.Verbose = true
	}
	if config.CPUProfileFile != "" {
		profile(config.CPUProfileFile)
	}

	if config.DataDir == "" {
		elog.Fatal("The data dir was not set and could not be guessed from machine name")
	}

	// Create data directory if it doesn't already exist.
	if err := os.MkdirAll(config.DataDir, 0744); err != nil {
		elog.Fatalf("Unable to create path: %s", err)
	}

	// Warn people if they have an info file
	info := filepath.Join(config.DataDir, "info")
	if _, err := os.Stat(info); err == nil {
		elog.Warnf("All cached configuration is now ignored. The file %s can be removed.", info)
	}

	var mbName string
	if config.Trace() {
		mbName = config.MetricsBucketName()
		runtime.SetBlockProfileRate(1)
	}

	mb := metrics.NewBucket(mbName)

	if config.GraphiteHost != "" {
		err := mb.Publish(config.GraphiteHost)
		if err != nil {
			panic(err)
		}
	}

	// Retrieve CORS configuration
	corsInfo, err := ehttp.NewCORSInfo(config.CorsOrigins)
	if err != nil {
		elog.Fatal("CORS:", err)
	}

	// Create etcd key-value store and registry.
	k.store = store.New()
	registry := server.NewRegistry(k.store)

	// Create stats objects
	followersStats := server.NewRaftFollowersStats(config.Name)
	serverStats := server.NewRaftServerStats(config.Name)

	// Calculate all of our timeouts
	heartbeatTimeout := time.Duration(config.Peer.HeartbeatTimeout) * time.Millisecond
	electionTimeout := time.Duration(config.Peer.ElectionTimeout) * time.Millisecond
	dialTimeout := (3 * heartbeatTimeout) + electionTimeout
	responseHeaderTimeout := (3 * heartbeatTimeout) + electionTimeout

	// Create peer server
	psConfig := server.PeerServerConfig{
		Name:           config.Name,
		Scheme:         config.PeerTLSInfo().Scheme(),
		URL:            config.Peer.Addr,
		SnapshotCount:  config.SnapshotCount,
		MaxClusterSize: config.MaxClusterSize,
		RetryTimes:     config.MaxRetryAttempts,
		RetryInterval:  config.RetryInterval,
	}
	ps := server.NewPeerServer(psConfig, registry, k.store, &mb, followersStats, serverStats)

	var psListener net.Listener = k.psListener
	if psConfig.Scheme == "https" {
		peerServerTLSConfig, err := config.PeerTLSInfo().ServerConfig()
		if err != nil {
			elog.Fatal("peer server TLS error: ", err)
		}

		psListener, err = server.NewTLSListener(config.Peer.BindAddr, peerServerTLSConfig)
		if err != nil {
			elog.Fatal("Failed to create peer listener: ", err)
		}
	} else {
		psListener, err = server.NewListener(config.Peer.BindAddr)
		if err != nil {
			elog.Fatal("Failed to create peer listener: ", err)
		}
	}

	// Create raft transporter and server
	raftTransporter := server.NewTransporter(followersStats, serverStats, registry, heartbeatTimeout, dialTimeout, responseHeaderTimeout)
	if psConfig.Scheme == "https" {
		raftClientTLSConfig, err := config.PeerTLSInfo().ClientConfig()
		if err != nil {
			elog.Fatal("raft client TLS error: ", err)
		}
		raftTransporter.SetTLSConfig(*raftClientTLSConfig)
	}
	raftServer, err := raft.NewServer(config.Name, config.DataDir, raftTransporter, k.store, ps, "")
	if err != nil {
		elog.Fatal(err)
	}
	raftServer.SetElectionTimeout(electionTimeout)
	raftServer.SetHeartbeatInterval(heartbeatTimeout)
	ps.SetRaftServer(raftServer)

	// Create etcd server
	s := server.New(config.Name, config.Addr, ps, registry, k.store, &mb)

	if config.Trace() {
		s.EnableTracing()
	}

	var sListener net.Listener = k.sListener
	if config.EtcdTLSInfo().Scheme() == "https" {
		etcdServerTLSConfig, err := config.EtcdTLSInfo().ServerConfig()
		if err != nil {
			elog.Fatal("etcd TLS error: ", err)
		}

		sListener, err = server.NewTLSListener(config.BindAddr, etcdServerTLSConfig)
		if err != nil {
			elog.Fatal("Failed to create TLS etcd listener: ", err)
		}
	} else {
		sListener, err = server.NewListener(config.BindAddr)
		if err != nil {
			elog.Fatal("Failed to create etcd listener: ", err)
		}
	}

	ps.SetServer(s)
	ps.Start(config.Snapshot, config.Peers)

	go func() {
		elog.Infof("peer server [name %s, listen on %s, advertised url %s]", ps.Config.Name, psListener.Addr(), ps.Config.URL)
		sHTTP := &ehttp.CORSHandler{ps.HTTPHandler(), corsInfo}
		elog.Fatal(http.Serve(psListener, sHTTP))
	}()

	elog.Infof("etcd server [name %s, listen on %s, advertised url %s]", s.Name, sListener.Addr(), s.URL())
	sHTTP := &ehttp.CORSHandler{s.HTTPHandler(), corsInfo}
	go func() { elog.Fatal(http.Serve(sListener, sHTTP)) }()

	close(ready)
}

// profile starts CPU profiling.
func profile(path string) {
	f, err := os.Create(path)
	if err != nil {
		elog.Fatal(err)
	}
	pprof.StartCPUProfile(f)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		sig := <-c
		elog.Infof("captured %v, stopping profiler and exiting..", sig)
		pprof.StopCPUProfile()
		os.Exit(1)
	}()
}
