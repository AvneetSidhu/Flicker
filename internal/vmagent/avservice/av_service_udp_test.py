import socket

sender = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)

receiver = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
receiver.bind(("127.0.0.1", 2048))


sender.sendto(b"Hello", ("127.0.0.1", 5000))

data, addr = receiver.recvfrom(2048)
print(f"Received: {data}")