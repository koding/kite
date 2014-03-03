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
	"path/filepath"
	"time"

	"github.com/coreos/etcd/config"
	ehttp "github.com/coreos/etcd/http"
	"github.com/coreos/etcd/metrics"
	"github.com/coreos/etcd/server"
	"github.com/coreos/etcd/store"
	"github.com/coreos/etcd/third_party/github.com/coreos/raft"
)

// This function is copied and modified from github.com/coreos/etcd/main.go file.
func (k *Kontrol) runEtcd(ready chan bool) {
	// Load config values from kontrol.
	var config = config.New()
	config.Name = k.Name       // name of the etcd instance
	config.DataDir = k.DataDir // directory to store etcd log
	config.Peers = k.Peers     // comma seperated values of other peers

	// Load other defaults.
	config.Load(nil)

	config.BindAddr = k.EtcdBindAddr
	config.Addr = k.EtcdAddr
	config.Peer.BindAddr = k.PeerBindAddr
	config.Peer.Addr = k.PeerAddr

	if config.DataDir == "" {
		log.Fatal("The data dir was not set and could not be guessed from machine name")
	}

	// Create data directory if it doesn't already exist.
	if err := os.MkdirAll(config.DataDir, 0744); err != nil {
		log.Fatal("Unable to create path: %s", err)
	}

	// Warn people if they have an info file
	info := filepath.Join(config.DataDir, "info")
	if _, err := os.Stat(info); err == nil {
		log.Warning("All cached configuration is now ignored. The file %s can be removed.", info)
	}

	var mbName string

	mb := metrics.NewBucket(mbName)

	// Retrieve CORS configuration
	corsInfo, err := ehttp.NewCORSInfo(config.CorsOrigins)
	if err != nil {
		log.Fatal("CORS:", err)
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
			log.Fatal("peer server TLS error: ", err)
		}

		psListener, err = server.NewTLSListener(config.Peer.BindAddr, peerServerTLSConfig)
		if err != nil {
			log.Fatal("Failed to create peer listener: ", err)
		}
	} else {
		psListener, err = server.NewListener(config.Peer.BindAddr)
		if err != nil {
			log.Fatal("Failed to create peer listener: ", err)
		}
	}

	// Create raft transporter and server
	raftTransporter := server.NewTransporter(followersStats, serverStats, registry, heartbeatTimeout, dialTimeout, responseHeaderTimeout)
	if psConfig.Scheme == "https" {
		raftClientTLSConfig, err := config.PeerTLSInfo().ClientConfig()
		if err != nil {
			log.Fatal("raft client TLS error: ", err)
		}
		raftTransporter.SetTLSConfig(*raftClientTLSConfig)
	}
	raftServer, err := raft.NewServer(config.Name, config.DataDir, raftTransporter, k.store, ps, "")
	if err != nil {
		log.Fatal(err.Error())
	}
	raftServer.SetElectionTimeout(electionTimeout)
	raftServer.SetHeartbeatInterval(heartbeatTimeout)
	ps.SetRaftServer(raftServer)

	// Create etcd server
	s := server.New(config.Name, config.Addr, ps, registry, k.store, &mb)

	var sListener net.Listener = k.sListener
	if config.EtcdTLSInfo().Scheme() == "https" {
		etcdServerTLSConfig, err := config.EtcdTLSInfo().ServerConfig()
		if err != nil {
			log.Fatal("etcd TLS error: ", err)
		}

		sListener, err = server.NewTLSListener(config.BindAddr, etcdServerTLSConfig)
		if err != nil {
			log.Fatal("Failed to create TLS etcd listener: ", err)
		}
	} else {
		sListener, err = server.NewListener(config.BindAddr)
		if err != nil {
			log.Fatal("Failed to create etcd listener: ", err)
		}
	}

	ps.SetServer(s)
	ps.Start(config.Snapshot, config.Peers)

	go func() {
		log.Info("peer server [name %s, listen on %s, advertised url %s]", ps.Config.Name, psListener.Addr(), ps.Config.URL)
		sHTTP := &ehttp.CORSHandler{ps.HTTPHandler(), corsInfo}
		if err := http.Serve(psListener, sHTTP); err != nil {
			log.Fatal(err.Error())
		}
	}()

	log.Info("etcd server [name %s, listen on %s, advertised url %s]", s.Name, sListener.Addr(), s.URL())
	sHTTP := &ehttp.CORSHandler{s.HTTPHandler(), corsInfo}
	go func() {
		if err := http.Serve(sListener, sHTTP); err != nil {
			log.Fatal(err.Error())
		}
	}()

	close(ready)
}
