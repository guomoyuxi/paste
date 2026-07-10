APP_NAME := paste
VERSION := 1.0.0
BUILD_DIR := build
APP_BUNDLE := $(BUILD_DIR)/Paste.app

# 架构相关变量（由 arch 目标设置）
GOARCH :=
ARCH_SUFFIX :=

.PHONY: build app dmg clean run tidy release arm64 amd64

# Intel (amd64)
amd64:
	$(MAKE) build GOARCH=amd64 ARCH_SUFFIX=intel
	$(MAKE) app GOARCH=amd64 ARCH_SUFFIX=intel
	$(MAKE) dmg GOARCH=amd64 ARCH_SUFFIX=intel

# M 系列 (arm64)
arm64:
	$(MAKE) build GOARCH=arm64 ARCH_SUFFIX=arm64
	$(MAKE) app GOARCH=arm64 ARCH_SUFFIX=arm64
	$(MAKE) dmg GOARCH=arm64 ARCH_SUFFIX=arm64

# 一键构建两个架构的 DMG
release: amd64 arm64
	@echo ""
	@echo "===== 构建完成 ====="
	@ls -lh $(BUILD_DIR)/Paste-*-*.dmg 2>/dev/null

build:
	@mkdir -p $(BUILD_DIR)
	@if [ "$(GOARCH)" = "arm64" ]; then \
		echo "交叉编译 arm64..."; \
		CGO_ENABLED=1 GOARCH=arm64 CC="clang -target arm64-apple-darwin" go build -o $(BUILD_DIR)/$(APP_NAME)-$(ARCH_SUFFIX) .; \
	else \
		echo "编译 amd64..."; \
		CGO_ENABLED=1 GOARCH=amd64 CC=clang go build -o $(BUILD_DIR)/$(APP_NAME)-$(ARCH_SUFFIX) .; \
	fi
	@echo "构建完成: $(BUILD_DIR)/$(APP_NAME)-$(ARCH_SUFFIX)"

app: build
	@rm -rf $(APP_BUNDLE)-$(ARCH_SUFFIX)
	@mkdir -p $(APP_BUNDLE)-$(ARCH_SUFFIX)/Contents/MacOS
	@mkdir -p $(APP_BUNDLE)-$(ARCH_SUFFIX)/Contents/Resources
	cp $(BUILD_DIR)/$(APP_NAME)-$(ARCH_SUFFIX) $(APP_BUNDLE)-$(ARCH_SUFFIX)/Contents/MacOS/paste
	cp Info.plist $(APP_BUNDLE)-$(ARCH_SUFFIX)/Contents/
	cp assets/AppIcon.icns $(APP_BUNDLE)-$(ARCH_SUFFIX)/Contents/Resources/
	@echo "应用包构建完成: $(APP_BUNDLE)-$(ARCH_SUFFIX)"

dmg: app
	@DMG_NAME=Paste-$(VERSION)-$(ARCH_SUFFIX).dmg; \
	DMG_PATH=$(BUILD_DIR)/$$DMG_NAME; \
	rm -f $$DMG_PATH; \
	mkdir -p $(BUILD_DIR)/dmg_temp_$(ARCH_SUFFIX); \
	rm -rf $(BUILD_DIR)/dmg_temp_$(ARCH_SUFFIX)/*; \
	cp -R $(APP_BUNDLE)-$(ARCH_SUFFIX) $(BUILD_DIR)/dmg_temp_$(ARCH_SUFFIX)/Paste.app; \
	ln -sf /Applications $(BUILD_DIR)/dmg_temp_$(ARCH_SUFFIX)/Applications; \
	hdiutil create -volname "Paste" -srcfolder $(BUILD_DIR)/dmg_temp_$(ARCH_SUFFIX) -ov -format UDZO $$DMG_PATH; \
	rm -rf $(BUILD_DIR)/dmg_temp_$(ARCH_SUFFIX); \
	echo "DMG 安装包构建完成: $$DMG_PATH"
	@# 去除隔离属性，避免 macOS Gatekeeper 标记为"已损坏"
	xattr -cr $(BUILD_DIR)/Paste-$(VERSION)-$(ARCH_SUFFIX).dmg
	@echo "已清除 DMG 隔离属性"

clean:
	rm -rf $(BUILD_DIR)
	rm -rf ~/Library/Application\ Support/Paste/paste.db

run: app
	open $(APP_BUNDLE)-$(ARCH_SUFFIX)

tidy:
	go mod tidy
