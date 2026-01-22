# SPDX-License-Identifier: AGPL-3.0-only
#
# Copyright (C) 2025 Rex

include $(TOPDIR)/rules.mk

PKG_NAME:=nanohat-oled
PKG_VERSION:=1.0.0
PKG_RELEASE:=1

PKG_SOURCE_PROTO:=git
PKG_SOURCE_URL:=https://github.com/rexac/NanoHatOLED.git
PKG_SOURCE_VERSION:=main
PKG_SOURCE_SUBDIR:=$(PKG_NAME)-$(PKG_VERSION)
PKG_BUILD_DIR:=$(BUILD_DIR)/$(PKG_SOURCE_SUBDIR)

PKG_LICENSE:=AGPL-3.0-only
PKG_LICENSE_FILES:=LICENSE
PKG_MAINTAINER:=Rex

PKG_BUILD_DEPENDS:=golang/host
PKG_BUILD_PARALLEL:=1
PKG_BUILD_FLAGS:=no-mips16

GO_PKG:=nanohat-oled
GO_PKG_LDFLAGS_X:=$(GO_PKG)/version.Version=$(PKG_VERSION)

include $(INCLUDE_DIR)/package.mk
include $(TOPDIR)/feeds/packages/lang/golang/golang-package.mk

define Package/nanohat-oled
  SECTION:=utils
  CATEGORY:=Utilities
  TITLE:=NanoHatOLED display control tool for OpenWRT
  URL:=https://github.com/rexac/NanoHatOLED
  DEPENDS:=$(GO_ARCH_DEPENDS) +i2c-tools +kmod-i2c-core +kmod-i2c-gpio
endef

define Package/nanohat-oled/description
  Go language driver for NanoHatOLED (I2C OLED display), adapted for OpenWRT systems.
endef

ifneq ($(CONFIG_USE_MUSL),)
  TARGET_CFLAGS += -D_LARGEFILE64_SOURCE
endif

define Package/nanohat-oled/install
	$(call GoPackage/Package/Install/Bin,$(PKG_INSTALL_DIR))

	$(INSTALL_DIR) $(1)/etc/NanoHatOLED
	$(INSTALL_BIN) $(PKG_INSTALL_DIR)/usr/bin/nanohat-oled $(1)/etc/NanoHatOLED/nanohat-oled
	$(CP) $(PKG_BUILD_DIR)/files/NanoHatOLED/* $(1)/etc/NanoHatOLED

	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) $(PKG_BUILD_DIR)/files/nanohatoled.init $(1)/etc/init.d/nanohatoled
endef

define Package/$(PKG_NAME)/postinst
#!/bin/sh
if [ -z "$${IPKG_INSTROOT}" ]; then
	/etc/init.d/nanohatoled enable
fi
exit 0
endef

$(eval $(call GoBinPackage,nanohat-oled))
$(eval $(call BuildPackage,nanohat-oled))