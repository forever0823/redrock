#!/bin/bash

set +e

detect_os_family() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        case "$ID" in
            ubuntu|debian) echo "debian"; return ;;
            centos|rhel|rocky|almalinux|fedora) echo "redhat"; return ;;
        esac
    fi
    if command -v apt-get >/dev/null 2>&1; then
        echo "debian"
        return
    fi
    if command -v yum >/dev/null 2>&1 || command -v dnf >/dev/null 2>&1; then
        echo "redhat"
        return
    fi
    echo "unknown"
}

detect_arch() {
    arch="$(uname -m 2>/dev/null)"
    case "$arch" in
        x86_64|amd64) echo "x86_64" ;;
        aarch64|arm64) echo "arm64" ;;
        *) echo "$arch" ;;
    esac
}

install_cmd() {
    cmd="$1"
    os_family="$(detect_os_family)"

    if [ "$os_family" = "debian" ]; then
        sudo apt-get update -y >/dev/null 2>&1
        case "$cmd" in
            bc) sudo apt-get install -y bc ;;
            ip) sudo apt-get install -y iproute2 ;;
            netstat) sudo apt-get install -y net-tools ;;
            free) sudo apt-get install -y procps ;;
            pgrep|ps|top) sudo apt-get install -y procps ;;
            *) sudo apt-get install -y "$cmd" ;;
        esac
        return $?
    fi

    if [ "$os_family" = "redhat" ]; then
        pkg_tool="yum"
        command -v dnf >/dev/null 2>&1 && pkg_tool="dnf"
        case "$cmd" in
            bc) sudo "$pkg_tool" install -y bc ;;
            ip) sudo "$pkg_tool" install -y iproute ;;
            netstat) sudo "$pkg_tool" install -y net-tools ;;
            free) sudo "$pkg_tool" install -y procps-ng ;;
            pgrep|ps|top) sudo "$pkg_tool" install -y procps-ng ;;
            *) sudo "$pkg_tool" install -y "$cmd" ;;
        esac
        return $?
    fi

    return 1
}

ensure_cmd() {
    cmd="$1"
    if command -v "$cmd" >/dev/null 2>&1; then
        return 0
    fi
    echo "[WARNING] 命令缺失: $cmd，尝试自动安装..."
    install_cmd "$cmd"
    if command -v "$cmd" >/dev/null 2>&1; then
        echo "[INFO] 命令安装成功: $cmd"
        return 0
    fi
    echo "[ERROR] 命令安装失败: $cmd"
    return 1
}

ensure_cmds() {
    for c in "$@"; do
        ensure_cmd "$c" || return 1
    done
    return 0
}
