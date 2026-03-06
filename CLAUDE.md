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

- right now I am returning to this project so we'll need to review what we have so far to see if it meets requirements
- I know the frontend and backend are not connected properly yet
- there are a lot of unfinished features lets document whats not done and put it in txt file.