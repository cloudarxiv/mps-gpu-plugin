/*
 * Copyright (c) 2019, NVIDIA CORPORATION.  All rights reserved.
 * Copyright 2023 Nebuly.com.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	spec "github.com/xzaviourr/k8s-device-plugin/api/config/v1"
	"github.com/xzaviourr/k8s-device-plugin/internal/rm"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

// Constants for use by the 'volume-mounts' device list strategy
const (
	deviceListAsVolumeMountsHostPath          = "/dev/null"
	deviceListAsVolumeMountsContainerPathRoot = "/var/run/nvidia-container-devices"
)

// NvidiaDevicePlugin implements the Kubernetes device plugin API
type NvidiaDevicePlugin struct {
	rm               rm.ResourceManager
	config           *spec.Config
	deviceListEnvvar string
	socket           string

	server *grpc.Server
	health chan *rm.Device
	stop   chan interface{}
}

// NewNvidiaDevicePlugin returns an initialized NvidiaDevicePlugin
func NewNvidiaDevicePlugin(config *spec.Config, resourceManager rm.ResourceManager) *NvidiaDevicePlugin {
	_, name := resourceManager.Resource().Split()

	return &NvidiaDevicePlugin{
		rm:               resourceManager,
		config:           config,
		deviceListEnvvar: "NVIDIA_VISIBLE_DEVICES",
		socket:           pluginapi.DevicePluginPath + "nvidia-" + name + ".sock",

		// These will be reinitialized every
		// time the plugin server is restarted.
		server: nil,
		health: nil,
		stop:   nil,
	}
}

func (plugin *NvidiaDevicePlugin) initialize() {
	plugin.server = grpc.NewServer([]grpc.ServerOption{}...)
	plugin.health = make(chan *rm.Device)
	plugin.stop = make(chan interface{})
}

func (plugin *NvidiaDevicePlugin) cleanup() {
	close(plugin.stop)
	plugin.server = nil
	plugin.health = nil
	plugin.stop = nil
}

// Devices returns the full set of devices associated with the plugin.
func (plugin *NvidiaDevicePlugin) Devices() rm.Devices {
	return plugin.rm.Devices()
}

// Start starts the gRPC server, registers the device plugin with the Kubelet,
// and starts the device healthchecks.
func (plugin *NvidiaDevicePlugin) Start() error {
	plugin.initialize()

	err := plugin.Serve()
	if err != nil {
		log.Printf("Could not start device plugin for '%s': %s", plugin.rm.Resource(), err)
		plugin.cleanup()
		return err
	}
	log.Printf("Starting to serve '%s' on %s", plugin.rm.Resource(), plugin.socket)

	err = plugin.Register()
	if err != nil {
		log.Printf("Could not register device plugin: %s", err)
		plugin.Stop()
		return err
	}
	log.Printf("Registered device plugin for '%s' with Kubelet", plugin.rm.Resource())

	go func() {
		err := plugin.rm.CheckHealth(plugin.stop, plugin.health)
		if err != nil {
			log.Printf("Failed to start health check: %v; continuing with health checks disabled", err)
		}
	}()

	return nil
}

// Stop stops the gRPC server.
func (plugin *NvidiaDevicePlugin) Stop() error {
	if plugin == nil || plugin.server == nil {
		return nil
	}
	log.Printf("Stopping to serve '%s' on %s", plugin.rm.Resource(), plugin.socket)
	plugin.server.Stop()
	if err := os.Remove(plugin.socket); err != nil && !os.IsNotExist(err) {
		return err
	}
	plugin.cleanup()
	return nil
}

// Serve starts the gRPC server of the device plugin.
func (plugin *NvidiaDevicePlugin) Serve() error {
	os.Remove(plugin.socket)
	sock, err := net.Listen("unix", plugin.socket)
	if err != nil {
		return err
	}

	pluginapi.RegisterDevicePluginServer(plugin.server, plugin)

	go func() {
		lastCrashTime := time.Now()
		restartCount := 0
		for {
			log.Printf("Starting GRPC server for '%s'", plugin.rm.Resource())
			err := plugin.server.Serve(sock)
			if err == nil {
				break
			}

			log.Printf("GRPC server for '%s' crashed with error: %v", plugin.rm.Resource(), err)

			// restart if it has not been too often
			// i.e. if server has crashed more than 5 times and it didn't last more than one hour each time
			if restartCount > 5 {
				// quit
				log.Fatalf("GRPC server for '%s' has repeatedly crashed recently. Quitting", plugin.rm.Resource())
			}
			timeSinceLastCrash := time.Since(lastCrashTime).Seconds()
			lastCrashTime = time.Now()
			if timeSinceLastCrash > 3600 {
				// it has been one hour since the last crash.. reset the count
				// to reflect on the frequency
				restartCount = 1
			} else {
				restartCount++
			}
		}
	}()

	// Wait for server to start by launching a blocking connexion
	conn, err := plugin.dial(plugin.socket, 5*time.Second)
	if err != nil {
		return err
	}
	conn.Close()

	return nil
}

// Register registers the device plugin for the given resourceName with Kubelet.
func (plugin *NvidiaDevicePlugin) Register() error {
	conn, err := plugin.dial(pluginapi.KubeletSocket, 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pluginapi.NewRegistrationClient(conn)
	reqt := &pluginapi.RegisterRequest{
		Version:      pluginapi.Version,
		Endpoint:     path.Base(plugin.socket),
		ResourceName: string(plugin.rm.Resource()),
		Options: &pluginapi.DevicePluginOptions{
			GetPreferredAllocationAvailable: true,
		},
	}

	_, err = client.Register(context.Background(), reqt)
	if err != nil {
		return err
	}
	return nil
}

// GetDevicePluginOptions returns the values of the optional settings for this plugin
func (plugin *NvidiaDevicePlugin) GetDevicePluginOptions(context.Context, *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	options := &pluginapi.DevicePluginOptions{
		GetPreferredAllocationAvailable: true,
	}
	return options, nil
}

// ListAndWatch lists devices and update that list according to the health status
func (plugin *NvidiaDevicePlugin) ListAndWatch(e *pluginapi.Empty, s pluginapi.DevicePlugin_ListAndWatchServer) error {
	s.Send(&pluginapi.ListAndWatchResponse{Devices: plugin.apiDevices()})

	for {
		select {
		case <-plugin.stop:
			return nil
		case d := <-plugin.health:
			// FIXME: there is no way to recover from the Unhealthy state.
			d.Health = pluginapi.Unhealthy
			log.Printf("'%s' device marked unhealthy: %s", plugin.rm.Resource(), d.ID)
			s.Send(&pluginapi.ListAndWatchResponse{Devices: plugin.apiDevices()})
		}
	}
}

// GetPreferredAllocation returns the preferred allocation from the set of devices specified in the request
func (plugin *NvidiaDevicePlugin) GetPreferredAllocation(ctx context.Context, r *pluginapi.PreferredAllocationRequest) (*pluginapi.PreferredAllocationResponse, error) {
	response := &pluginapi.PreferredAllocationResponse{}
	for _, req := range r.ContainerRequests {
		devices, err := plugin.rm.GetPreferredAllocation(req.AvailableDeviceIDs, req.MustIncludeDeviceIDs, int(req.AllocationSize))
		if err != nil {
			return nil, fmt.Errorf("error getting list of preferred allocation devices: %v", err)
		}

		resp := &pluginapi.ContainerPreferredAllocationResponse{
			DeviceIDs: devices,
		}

		response.ContainerResponses = append(response.ContainerResponses, resp)
	}
	return response, nil
}

// Allocate which return list of devices.
func (plugin *NvidiaDevicePlugin) Allocate(ctx context.Context, reqs *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	responses := pluginapi.AllocateResponse{}
	for _, req := range reqs.ContainerRequests {
		// If the devices being allocated are replicas, then (conditionally)
		// error out if more than one resource is being allocated.
		if plugin.config.Sharing.TimeSlicing.FailRequestsGreaterThanOne && rm.AnnotatedIDs(req.DevicesIDs).AnyHasAnnotations() {
			if len(req.DevicesIDs) > 1 {
				return nil, fmt.Errorf("request for '%v: %v' too large: maximum request size for shared resources is 1", plugin.rm.Resource(), len(req.DevicesIDs))
			}
		}

		for _, id := range req.DevicesIDs {
			if !plugin.rm.Devices().Contains(id) {
				return nil, fmt.Errorf("invalid allocation request for '%s': unknown device: %s", plugin.rm.Resource(), id)
			}
		}

		response := pluginapi.ContainerAllocateResponse{}

		ids := req.DevicesIDs
		mpsDevices := plugin.getMPSDevices()
		requestedMPSDevices := mpsDevices.Subset(ids)
		deviceIDs := plugin.deviceIDsFromAnnotatedDeviceIDs(ids)

		// If the devices being allocated are replicas, then (conditionally)
		// error out if more than one resource is being allocated.
		if plugin.config.Sharing.MPS.FailRequestsGreaterThanOne && len(requestedMPSDevices) > 1 {
			return nil, fmt.Errorf("request for '%v: %v' too large: maximum request size for shared resources is 1", plugin.rm.Resource(), len(req.DevicesIDs))
		}

		if *plugin.config.Flags.Plugin.DeviceListStrategy == spec.DeviceListStrategyEnvvar {
			response.Envs = plugin.apiEnvs(plugin.deviceListEnvvar, deviceIDs)
		}
		if *plugin.config.Flags.Plugin.DeviceListStrategy == spec.DeviceListStrategyVolumeMounts {
			response.Envs = plugin.apiEnvs(plugin.deviceListEnvvar, []string{deviceListAsVolumeMountsContainerPathRoot})
			response.Mounts = plugin.apiMounts(deviceIDs)
		}
		if *plugin.config.Flags.Plugin.PassDeviceSpecs {
			response.Devices = plugin.apiDeviceSpecs(*plugin.config.Flags.NvidiaDriverRoot, ids)
		}
		if *plugin.config.Flags.GDSEnabled {
			response.Envs["NVIDIA_GDS"] = "enabled"
		}
		if *plugin.config.Flags.MOFEDEnabled {
			response.Envs["NVIDIA_MOFED"] = "enabled"
		}

		if len(requestedMPSDevices) == 0 {
			responses.ContainerResponses = append(responses.ContainerResponses, &response)
			continue
		}

		// Configure MPS devices
		log.Printf("configuring requested MPS devices: %+v", requestedMPSDevices)
		if response.Mounts == nil {
			response.Mounts = make([]*pluginapi.Mount, 0)
		}
		if response.Envs == nil {
			response.Envs = make(map[string]string)
		}

		memLimits := make([]string, 0)
		for _, mpsDevice := range requestedMPSDevices {
			limit := fmt.Sprintf("%s=%dG", mpsDevice.Index, mpsDevice.AnnotatedID.GetPartition())
			memLimits = append(memLimits, limit)
		}
		response.Envs["CUDA_MPS_PINNED_DEVICE_MEM_LIMIT"] = strings.Join(memLimits, ",")
		response.Envs["CUDA_MPS_PIPE_DIRECTORY"] = "/tmp/nvidia-mps"
		mount := pluginapi.Mount{
			ContainerPath: "/tmp/nvidia-mps",
			HostPath:      "/tmp/nvidia-mps",
		}
		response.Mounts = append(response.Mounts, &mount)
		responses.ContainerResponses = append(responses.ContainerResponses, &response)
	}

	return &responses, nil
}

// PreStartContainer is unimplemented for this plugin
func (plugin *NvidiaDevicePlugin) PreStartContainer(context.Context, *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return &pluginapi.PreStartContainerResponse{}, nil
}

// dial establishes the gRPC communication with the registered device plugin.
func (plugin *NvidiaDevicePlugin) dial(unixSocketPath string, timeout time.Duration) (*grpc.ClientConn, error) {
	c, err := grpc.Dial(unixSocketPath, grpc.WithInsecure(), grpc.WithBlock(),
		grpc.WithTimeout(timeout),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}),
	)

	if err != nil {
		return nil, err
	}

	return c, nil
}

func (plugin *NvidiaDevicePlugin) deviceIDsFromAnnotatedDeviceIDs(ids []string) []string {
	var deviceIDs []string
	if *plugin.config.Flags.Plugin.DeviceIDStrategy == spec.DeviceIDStrategyUUID {
		deviceIDs = rm.AnnotatedIDs(ids).GetIDs()
	}
	if *plugin.config.Flags.Plugin.DeviceIDStrategy == spec.DeviceIDStrategyIndex {
		deviceIDs = plugin.rm.Devices().Subset(ids).GetIndices()
	}
	return deviceIDs
}

func (plugin *NvidiaDevicePlugin) apiDevices() []*pluginapi.Device {
	return plugin.rm.Devices().GetPluginDevices()
}

func (plugin *NvidiaDevicePlugin) apiEnvs(envvar string, deviceIDs []string) map[string]string {
	return map[string]string{
		envvar: strings.Join(deviceIDs, ","),
	}
}

func (plugin *NvidiaDevicePlugin) apiMounts(deviceIDs []string) []*pluginapi.Mount {
	var mounts []*pluginapi.Mount

	for _, id := range deviceIDs {
		mount := &pluginapi.Mount{
			HostPath:      deviceListAsVolumeMountsHostPath,
			ContainerPath: filepath.Join(deviceListAsVolumeMountsContainerPathRoot, id),
		}
		mounts = append(mounts, mount)
	}

	return mounts
}

func (plugin *NvidiaDevicePlugin) getMPSDevices() MPSDeviceList {
	var res = make(MPSDeviceList, 0)
	if plugin.config.Sharing.MPS.Resources == nil {
		return res
	}

	// Lookup table: keep track of MPS ids without annotations
	var mpsIDs = make(map[string]struct{})
	for _, r := range plugin.config.Sharing.MPS.Resources {
		for _, ref := range r.Devices {
			if ref.IsGPUIndex() {
				device := plugin.rm.Devices().GetByIndex(ref.String())
				if device != nil {
					// use ID without annotation
					id := rm.MPSAnnotatedID(device.GetID()).GetID()
					mpsIDs[id] = struct{}{}
				}
			}
			if ref.IsUUID() {
				mpsIDs[ref.String()] = struct{}{}
			}
		}
	}

	// Iterate over plugin devices and extract the MPS devices defined in config
	for _, d := range plugin.rm.Devices() {
		annotatedId := rm.MPSAnnotatedID(d.GetID())
		if _, ok := mpsIDs[annotatedId.GetID()]; ok {
			mpsDevice := MPSDevice{
				Index:       d.Index,
				AnnotatedID: annotatedId,
			}
			res = append(res, mpsDevice)
		}
	}

	return res
}

func (plugin *NvidiaDevicePlugin) apiDeviceSpecs(driverRoot string, ids []string) []*pluginapi.DeviceSpec {
	optional := map[string]bool{
		"/dev/nvidiactl":        true,
		"/dev/nvidia-uvm":       true,
		"/dev/nvidia-uvm-tools": true,
		"/dev/nvidia-modeset":   true,
	}

	paths := plugin.rm.GetDevicePaths(ids)

	var specs []*pluginapi.DeviceSpec
	for _, p := range paths {
		if optional[p] {
			if _, err := os.Stat(p); err != nil {
				continue
			}
		}
		spec := &pluginapi.DeviceSpec{
			ContainerPath: p,
			HostPath:      filepath.Join(driverRoot, p),
			Permissions:   "rw",
		}
		specs = append(specs, spec)
	}

	return specs
}
