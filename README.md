# Lanboat

**English** · [简体中文](README.zh-CN.md)

Lanboat is a desktop proxy client for Linux, Windows, and macOS. Import your proxy nodes or subscriptions, choose how traffic is routed, and manage DNS and connection logs from one app.

## Install

Download the installer for your operating system and processor from [GitHub Releases](https://github.com/zatxm/lanboat-app/releases). Choose `amd64` or `x86_64` for Intel and AMD PCs, and `arm64` or `aarch64` for 64-bit ARM. On macOS, choose `arm64` for Apple silicon or `amd64` for Intel.

| System | Package | Install |
| --- | --- | --- |
| Ubuntu, Debian, Linux Mint | `.deb` | `sudo apt install ./filename.deb` |
| Fedora | `.rpm` | `sudo dnf install ./filename.rpm` |
| openSUSE | `.rpm` | `sudo zypper install ./filename.rpm` |
| Arch Linux, Manjaro, EndeavourOS, Arch Linux ARM | `.pkg.tar.zst` | `sudo pacman -U ./filename.pkg.tar.zst` |
| Other glibc-based Linux distributions | `-linux-*.tar.gz` | Extract the archive and run `./install.sh` as your desktop user |
| Windows | `-setup.exe` | Run the installer and choose where to keep the app and your data |
| macOS | `.pkg` | Open the installer, then launch Lanboat from Applications |

On Linux, open Lanboat from your application menu as your desktop user. Lanboat needs an X11 or XWayland session and the X11 client library (`libX11.so.6`). File dialogs use Zenity. The archive installer expects you to install these dependencies with your distribution's package manager. Run `./uninstall.sh` from the extracted archive to remove that installation.

## Connect

1. On **Home**, select **Import** to paste node links or a subscription URL, or to upload a configuration file. Use **Add** to enter a node manually.
2. Select a node and choose a **Proxy mode**. **Rules** uses your routing configuration, **Global** sends traffic through the selected node, and **Direct** connects to destinations directly.
3. Turn on **Connect**. For apps that use a local proxy, use the address and port shown in **Settings → Proxy**.

Use **Latency test** to compare nodes. **Auto** selects the lowest-latency available node when connecting. On Linux, **Global + TUN** routes system TCP and UDP traffic through Lanboat. It requires an available TUN device, the `ip` and `pkexec` commands, and system authorization.

## Routing, DNS, and logs

- In **Settings → Proxy**, you can allow LAN devices to connect, require a username and password, or configure an upstream proxy.
- Open **Routing rules** in **Settings** to choose a routing configuration, manage custom rules, and adjust the region or update schedule. Subscription rules apply to the nodes from that subscription.
- Open **DNS settings** to change resolvers, run a query, or open the DNS leak test.
- In **Data**, turn on proxy or DNS logging and set an automatic cleanup interval. Proxy logs record each connection when it ends, with its destination, matched rule, outbound node, duration, and upload/download traffic; rejected or failed connections are recorded immediately, and plaintext HTTP requests also record the URL and method. Full request logging is off by default. It adds visible HTTP headers and up to 64 KiB of the body, which may contain cookies, tokens, or submitted data. HTTPS remains encrypted, so logs show the connection target for encrypted requests. The **App logs** tab shows Lanboat's own events, such as connecting, disconnecting, updates, and errors.

## Back up and find your data

Open **Settings → Backups** to save nodes, the active routing rules, or DNS settings separately. Backups may contain proxy credentials.

Use **Settings → About** to open the data folder. The default is `~/.lanboat` on Linux and macOS. Windows uses the folder chosen during installation, defaulting to `%USERPROFILE%\.lanboat`. Uninstalling the app keeps this folder.

[Third-party notices](THIRD_PARTY.md)
