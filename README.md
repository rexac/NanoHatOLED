# NanoHatOLED
OpenWrt OLED display for NanoHatOLED.

## Depends / 依赖
- i2c-tools
- kmod-i2c-core
- kmod-i2c-gpio

## Compile / 编译
```bash
# Add NanoHatOLED to OpenWrt package
# 将 NanoHatOLED 添加到 OpenWrt 软件包
mkdir -p openwrt/package/NanoHatOLED
wget -O openwrt/package/NanoHatOLED/Makefile https://github.com/rexac/NanoHatOLED/raw/main/Makefile

# Select this list item
# 选择要编译的包
# Extra utils -> nanohat-oled
make menuconfig
```
## Thanks / 谢致
- [friendlyarm/NanoHatOLED](https://github.com/friendlyarm/NanoHatOLED)
- [mmalcek/nanohatoled](https://github.com/mmalcek/nanohatoled)
- [vinewx/NanoHatOLED](https://github.com/vinewx/NanoHatOLED)

<img src="https://github.com/vinewx/NanoHatOLED/raw/master/assets/k1.jpg" width="250" /> <img src="https://github.com/vinewx/NanoHatOLED/raw/master/assets/k2.jpg" width="250" /> <img src="https://github.com/vinewx/NanoHatOLED/raw/master/assets/k3.jpg" width="250" />