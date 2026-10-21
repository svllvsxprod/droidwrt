<p align="center"><a href="README.md">English</a> · <a href="README.ru.md">Русский</a></p>
<h1 align="center">DroidWRT</h1>
<p align="center">Android-смартфон с root в роли переносного OpenWrt-роутера.</p>
<p align="center"><img alt="Android" src="https://img.shields.io/badge/Android-14b89a?style=flat-square"> <img alt="OpenWrt" src="https://img.shields.io/badge/OpenWrt-14b89a?style=flat-square"> <img alt="Go" src="https://img.shields.io/badge/Go-14b89a?style=flat-square"> <img alt="Noncommercial" src="https://img.shields.io/badge/Noncommercial-14b89a?style=flat-square"></p>

<p align="center"><img src="docs/readme/preview.png" width="360" alt="Anonymized demo preview"></p>
> Обезличенный демонстрационный кадр: брендинг и данные заменены с помощью AI. Иллюстрация интерфейса, не неизменённый скриншот.

## Возможности

| Возможность | Реализация |
| --- | --- |
| Единая панель | Нативная Android-панель состояния роутера, сетей и VPN-профилей. |
| Несколько подключений | Интеграция мобильной сети, Wi-Fi и раздачи через USB/Ethernet. |
| Маршрутизация VPN | Интеграция AmneziaWG и Mihomo, проверки связи, восстановление и kill switch. |
| Интеграция устройства | Сервис управления на Go, Android-помощник, патчи ядра и настройка SystemUI. |

## Исходники

| Каталог | Содержимое |
| --- | --- |
| `panel/` | Android-панель на Java. |
| `openwrt/` | Go-сервис, скрипты интеграции и модуль. |
| `android/ · system-ui/` | Интеграция с Android и системным интерфейсом. |
| `kernel/` | Сценарии сборки и патчи ядра для устройства. |

## Локальный запуск и проверка

```sh
cd openwrt
go test ./...
mkdir -p ../out
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o ../out/droidwrt-control .
```

Для Android-панели: JDK через `JAVA_HOME`, Android SDK через `ANDROID_HOME`, Android API 36 / Build Tools 36. Затем `python3 panel/build.py`. Сборка не устанавливает прошивку на устройство.

## Статус и границы проекта

Рабочая платформа — Redmi Note 8 Pro (begonia) с root и совместимым окружением Android, ядра и OpenWrt. Этот набор исходников не является универсальной прошивкой или установщиком в один клик. VPN-доступы, образы прошивок, дампы загрузки и приватная диагностика исключены. Для сборочных сценариев могут потребоваться внешние исходники и подготовка конкретного устройства.

## Лицензия

Собственный самостоятельный код распространяется по [PolyForm Noncommercial 1.0.0](LICENSE): коммерческое использование по этой лицензии не предоставляется. Условия не заменяют лицензии сторонних компонентов и производных патчей ядра. Подробности — [LICENSE-SCOPE.md](LICENSE-SCOPE.md).

---

<p align="center">
<a href="https://t.me/tribute/app?startapp=dK9j"><img src="docs/readme/support-tribute.svg" height="36" alt="Support via Tribute"></a>
<a href="https://nowpayments.io/donation/svllvsx"><img src="docs/readme/support-nowpayments.svg" height="36" alt="Support via NOWPayments"></a>
<a href="https://t.me/svllvsxprod"><img src="docs/readme/telegram.svg" height="36" alt="Telegram"></a>
</p>
