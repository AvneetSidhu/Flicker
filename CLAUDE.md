#Flicker - development context

## overview
- the way this app works is that a vmagent runs within a virtual machine and streams video / audio data to ports on the SFU application
- this video and audio data belongs to a certain room, we stream this data to the room and people within that room on the frontend will be able see the contents of the virtual machine
- client fanout is done over webrtc, signaling is done over websocket
- clients can forward their control information like keyboard and mouse input over websocket to the sfu and the sfu sends it over grpc to the vm
- from the frontend we are going to need to be able to provision the vms, which will launch from an image that already contains the vm agent baked in and ready to go off launch.

## Stack
- Go
- vite react
- database

## what to do

- right now we have it functioning so the frontend can trigger a new virtual machine to be spun up, we don't have the VM deletion made yet it needs to be implemented so we don't run out of disk space on the host
- we also have the issue of the frontend not receiving any audio / video from the vm agent
- we need to get a/v running before we move onto other stuff, right after we get vm cleanup and deletion worked out