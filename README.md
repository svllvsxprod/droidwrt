<p align="center"><a href="README.md">English</a> · <a href="README.ru.md">Русский</a></p>
<h1 align="center">DroidWRT</h1>
<p align="center">A rooted Android phone turned into a portable OpenWrt router.</p>
<p align="center"><img alt="Android" src="https://img.shields.io/badge/Android-14b89a?style=flat-square"> <img alt="OpenWrt" src="https://img.shields.io/badge/OpenWrt-14b89a?style=flat-square"> <img alt="Go" src="https://img.shields.io/badge/Go-14b89a?style=flat-square"> <img alt="Noncommercial" src="https://img.shields.io/badge/Noncommercial-14b89a?style=flat-square"></p>

<p align="center"><img src="docs/readme/preview.png" width="360" alt="Anonymized demo preview"></p>
> Anonymized presentation image: branding and data were replaced with AI. An interface illustration, not an unedited screenshot.

## Interface gallery

Individual views of the current interface. Addresses, profile names and network identity were replaced with demo values using AI.

### VPN and routes

<p><img src="docs/readme/demo-vpn.png" width="360" alt="VPN and routes"></p>

### Connection sharing

<p><img src="docs/readme/demo-sharing.png" width="360" alt="Connection sharing"></p>

### Wi-Fi settings

<p><img src="docs/readme/demo-wifi.png" width="360" alt="Wi-Fi settings"></p>

### Protection checks

<p><img src="docs/readme/demo-protection.png" width="360" alt="Protection checks"></p>

### Pocket lock

<p><img src="docs/readme/demo-pocket.png" width="360" alt="Pocket lock"></p>

## Highlights

| Feature | What is included |
| --- | --- |
| One control panel | A native Android panel for router status, networks and VPN profiles. |
| Multiple uplinks | Integration for mobile data, Wi-Fi and USB/Ethernet sharing. |
| VPN routing | AmneziaWG and Mihomo integration, health checks, recovery and kill-switch logic. |
| Device integration | Go control service, Android helper, kernel patches and SystemUI customization. |

## Source layout

| Path | Contents |
| --- | --- |
| `panel/` | Java Android control panel. |
| `openwrt/` | Go service, integration scripts and module. |
| `android/ · system-ui/` | Android integration and UI customization. |
| `kernel/` | Device-specific kernel build scripts and patches. |

## Local development

```sh
cd openwrt
go test ./...
mkdir -p ../out
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o ../out/droidwrt-control .
```

For the Android panel: provide a JDK through `JAVA_HOME`, Android SDK through `ANDROID_HOME`, and Android API 36 / Build Tools 36. Then run `python3 panel/build.py`. Building does not flash the device.

## Project status & scope

The working target is Redmi Note 8 Pro (begonia), with root and a compatible Android/kernel/OpenWrt environment. This source snapshot is not universal firmware or a one-click installer. VPN credentials, firmware images, boot dumps and private diagnostic captures are excluded. Build scripts may require external sources and device-specific prerequisites.

## License

Original standalone code is provided under [PolyForm Noncommercial 1.0.0](LICENSE); this license does not grant commercial use. It does not replace third-party licenses or the licenses of kernel-derived patches. See [LICENSE-SCOPE.md](LICENSE-SCOPE.md).

---

<p align="center">
<a href="https://t.me/tribute/app?startapp=dK9j"><img src="docs/readme/support-tribute.svg" height="36" alt="Support via Tribute"></a>
<a href="https://nowpayments.io/donation/svllvsx"><img src="docs/readme/support-nowpayments.svg" height="36" alt="Support via NOWPayments"></a>
<a href="https://t.me/svllvsxprod"><img src="docs/readme/telegram.svg" height="36" alt="Telegram"></a>
</p>
