"""Exercise the installed debug APK on a running Android emulator."""
import argparse
import http.server
import json
import re
import socket
import socketserver
import struct
import subprocess
import threading
import time
import xml.etree.ElementTree as ET

APP = "org.lanboat.mobile"
ACTIVITY = APP + "/org.fego.FegoActivity"
TARGET = "10.0.2.2"
MARKER = b"LANBOAT_VPN_SMOKE"


def wait_for(predicate, description, seconds=25):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        if value := predicate():
            return value
        time.sleep(0.5)
    raise RuntimeError(description)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("apk")
    parser.add_argument("screenshot")
    parser.add_argument("--serial")
    args = parser.parse_args()
    adb_cmd = ["adb"] + (["-s", args.serial] if args.serial else [])

    def adb(*command, data=None):
        return subprocess.check_output([*adb_cmd, *command], input=data, timeout=30)

    def vpn_active():
        output = adb("shell", "dumpsys", "connectivity")
        return re.search(rb"VPN CONNECTED extra: VPN:org\.lanboat\.mobile[}\s]", output)

    def connection_switch():
        # Find the grey switch track on the toolbar regardless of the label width.
        frame = adb("exec-out", "screencap")
        width, height, pixel_format = struct.unpack_from("<3I", frame)
        if (width, height, pixel_format) != (1080, 2340, 1):
            return False
        y = 145
        row = len(frame) - width * height * 4 + y * width * 4
        run = 0
        for x in range(408):
            offset = row + x * 4
            red, green, blue = frame[offset:offset + 3]
            run = run + 1 if red == green == blue == 229 else 0
            if run == 24:
                return str(x - run // 2), str(y)
        return None

    def permission_button():
        xml = "/data/local/tmp/lanboat-permission.xml"
        adb("shell", "uiautomator", "dump", xml)
        tree = ET.fromstring(adb("shell", "cat", xml))
        for node in tree.iter("node"):
            if node.get("resource-id") == "android:id/button1":
                left, top, right, bottom = map(int, re.findall(r"\d+", node.get("bounds")))
                return str((left + right) // 2), str((top + bottom) // 2)
        return None

    class Page(http.server.BaseHTTPRequestHandler):
        def do_GET(self):
            self.send_response(200)
            self.end_headers()
            self.wfile.write(MARKER)

    with http.server.ThreadingHTTPServer(("127.0.0.1", 0), Page) as page:
        target_port = page.server_address[1]
        forwarded = threading.Event()

        class Socks(socketserver.StreamRequestHandler):
            def handle(self):
                self.connection.settimeout(5)
                greeting = self.rfile.read(2)
                if len(greeting) != 2 or greeting[0] != 5:
                    return
                self.rfile.read(greeting[1])
                self.wfile.write(b"\x05\x00")
                header = self.rfile.read(4)
                if header != b"\x05\x01\x00\x01":
                    return
                host = socket.inet_ntoa(self.rfile.read(4))
                port, = struct.unpack("!H", self.rfile.read(2))
                if (host, port) != (TARGET, target_port):
                    self.wfile.write(b"\x05\x02\x00\x01" + bytes(6))
                    return
                with socket.create_connection(("127.0.0.1", port), timeout=5) as upstream:
                    self.wfile.write(b"\x05\x00\x00\x01" + bytes(6))
                    request = self.rfile.readline(4096)
                    while (line := self.rfile.readline(4096)) not in (b"\r\n", b""):
                        request += line
                    upstream.sendall(request + b"\r\n")
                    while response := upstream.recv(4096):
                        self.wfile.write(response)
                    forwarded.set()

        with socketserver.ThreadingTCPServer(("127.0.0.1", 0), Socks) as proxy:
            for server in (page, proxy):
                threading.Thread(target=server.serve_forever, daemon=True).start()
            adb("shell", "wm", "size", "1080x2340")
            adb("shell", "wm", "density", "420")
            adb("shell", "wm", "dismiss-keyguard")
            print(adb("install", "-r", args.apk).decode().strip(), flush=True)
            for mode in ("global", "rule"):
                forwarded.clear()
                adb("shell", "pm", "clear", APP)
                data = {
                    "version": 1,
                    "nodes": [{"id": "smoke", "name": "Local SOCKS5", "server": TARGET,
                               "port": proxy.server_address[1], "type": "socks5"}],
                    "selected": "smoke", "subscriptions": [], "rules": [],
                    "files": [{"id": "smoke", "name": "smoke.conf",
                               "content": f"[Rule]\nIP-CIDR,{TARGET}/32,PROXY,no-resolve\nFINAL,REJECT\n"}],
                    "activeFile": "smoke",
                    "settings": {"port": 1080, "mode": mode, "language": "en", "ruleAutoUpdate": False},
                }
                command = f"""run-as {APP} sh -c 'mkdir -p files/lanboat && cat > files/lanboat/config.json'"""
                adb("shell", "-T", command, data=json.dumps(data).encode())
                print(adb("shell", "am", "start", "-W", "-n", ACTIVITY).decode().strip(), flush=True)
                switch = wait_for(connection_switch, "Android did not draw the connection switch")
                adb("shell", "input", "tap", *switch)
                if not vpn_active():
                    button = wait_for(lambda: "active" if vpn_active() else permission_button(),
                                      "Android did not present VPN authorization")
                    if button != "active":
                        adb("shell", "input", "tap", *button)
                wait_for(vpn_active, "Android VPN did not connect")
                response = adb("shell", r'printf "GET /smoke HTTP/1.1\r\nHost: fixture\r\nConnection: close\r\n\r\n"'
                               f" | toybox nc -w 5 {TARGET} {target_port}")
                if MARKER not in response or not forwarded.wait(5):
                    raise RuntimeError("The HTTP response did not traverse the SOCKS5 proxy")
                if mode == "rule":
                    with open(args.screenshot, "wb") as image:
                        image.write(adb("exec-out", "screencap", "-p"))
                adb("shell", "input", "tap", *switch)
                wait_for(lambda: not vpn_active(), "Disconnect did not remove the VPN")
                print(f"Android VPN ({mode}) connected, forwarded HTTP through SOCKS5, and disconnected.", flush=True)


if __name__ == "__main__":
    main()
