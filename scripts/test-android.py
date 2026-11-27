"""Exercise the installed debug APK on a running Android emulator."""
import argparse
import http.server
import json
import re
import socket
import socketserver
import struct
import subprocess
import sys
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
        return subprocess.check_output([*adb_cmd, *command], input=data, stderr=subprocess.PIPE, timeout=30)

    def read_device(*command):
        try:
            return adb(*command)
        except subprocess.CalledProcessError as error:
            if not re.search(rb"no devices/emulators found|device (?:'[^']+' )?(?:not found|offline)", error.stderr):
                raise
            print("ADB transport interrupted. Waiting for the emulator to reconnect.", flush=True)
            adb("wait-for-device")
            return adb(*command)

    def vpn_active():
        output = read_device("shell", "dumpsys", "connectivity")
        return re.search(rb"VPN CONNECTED extra: VPN:org\.lanboat\.mobile[}\s]", output)

    def control_position(matches):
        xml = "/data/local/tmp/lanboat-ui.xml"
        if b"dumped to" not in read_device("shell", "uiautomator", "dump", xml):
            return None
        tree = ET.fromstring(read_device("shell", "cat", xml))
        for node in tree.iter("node"):
            if matches(node) and node.get("enabled") == "true":
                left, top, right, bottom = map(int, re.findall(r"-?\d+", node.get("bounds")))
                return str((left + right) // 2), str((top + bottom) // 2)
        return None

    def connection_switch(label):
        return control_position(lambda node: node.get("package") == APP
                                and node.get("class") == "android.widget.Switch"
                                and node.get("content-desc") == label)

    def permission_button():
        return control_position(lambda node: node.get("resource-id") == "android:id/button1")

    def content_repainted():
        raw = read_device("exec-out", "screencap")
        width, height, pixel_format = struct.unpack_from("<III", raw)
        if pixel_format not in (1, 2):
            raise RuntimeError(f"Unexpected Android screenshot format {pixel_format}")
        pixels = memoryview(raw)[-4 * width * height:]
        # Ignore the status and navigation bars, which are drawn even if Lanboat is blank.
        stride = 4 * width
        colors = {
            bytes(pixels[y * stride + 4 * x:y * stride + 4 * x + 3])
            for y in range(height // 12, height * 3 // 4, 8)
            for x in range(width // 20, width * 19 // 20, 8)
        }
        return len(colors) > 8

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
                switch = wait_for(lambda: connection_switch("Connect"), "Android did not expose the Connect switch")
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
                read_device("shell", "am", "start", "-W", "-a", "android.settings.SETTINGS")
                read_device("shell", "am", "start", "-W", "-n", ACTIVITY)
                wait_for(content_repainted, "Android returned to a blank Lanboat window")
                if not vpn_active():
                    raise RuntimeError("Android VPN disconnected while switching apps")
                if mode == "rule":
                    with open(args.screenshot, "wb") as image:
                        image.write(read_device("exec-out", "screencap", "-p"))
                switch = wait_for(lambda: connection_switch("Disconnect"), "Android did not expose the Disconnect switch")
                adb("shell", "input", "tap", *switch)
                wait_for(lambda: not vpn_active(), "Disconnect did not remove the VPN")
                print(f"Android VPN ({mode}) connected, forwarded HTTP through SOCKS5, and disconnected.", flush=True)


if __name__ == "__main__":
    try:
        main()
    except subprocess.CalledProcessError as error:
        sys.stderr.buffer.write(error.stderr)
        raise
