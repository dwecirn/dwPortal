# 原生部署（测试阶段）

当前使用 `script/deploy/manage.sh` 管理本机安装生命周期；这些能力后续会迁入 `dwportal` 二进制。脚本只管理 dwPortal 自身，不修改或重载 Caddy。

## 文件布局

- `/opt/dwportal/releases/<version>/dwportal`：不可变版本目录。
- `/opt/dwportal/current`：原子切换到当前版本的软链接。
- `/etc/systemd/system/dwportal.service`：systemd unit。
- `/var/lib/dwportal/catalog.yaml`：测试阶段软链接到仓库的 `config/config.yaml`。

因为 catalog 暂时位于用户 home 下，安装脚本会把 unit 的 `User`/`Group` 渲染为该配置文件的拥有者，并让 home 只读。配置迁往独立运行目录后，应恢复专用 `dwportal` 用户。

## 构建与安装

```bash
make package VERSION=0.2.0
sudo script/deploy/manage.sh install \
  --package dist/dwportal-0.2.0-linux-amd64.tar.gz
```

安装会创建版本目录、catalog 软链接、`current` 软链接，并启用 `dwportal.service`。重复安装会被拒绝。

## 升级与回滚

```bash
make package VERSION=0.2.1
sudo script/deploy/manage.sh upgrade \
  --package dist/dwportal-0.2.1-linux-amd64.tar.gz
```

升级不会覆盖旧版本。脚本先安装新 release，再原子切换 `current`；若 systemd 重启失败，会恢复之前的链接并尝试启动旧版本。

## 从 Caddyfile 同步测试配置

```bash
sudo script/deploy/manage.sh sync-config \
  --caddyfile /etc/caddy/Caddyfile \
  --base-domain dw.c
```

同步仅读取 Caddyfile，通过临时文件生成目录，再保持仓库配置原有所有者写入 `config/config.yaml`。生成结果保留默认监听端口 `6480`，后端会热加载服务条目；端口变化仍需重启服务。

## 卸载

```bash
sudo script/deploy/manage.sh uninstall
```

默认删除 unit 和 `/opt/dwportal`，保留 `/var/lib/dwportal`。确认不再需要运行状态时使用：

```bash
sudo script/deploy/manage.sh uninstall --purge
```

卸载不会删除仓库中的测试配置，也不会操作 Caddy。

## 无系统副作用测试

```bash
DWPORTAL_ROOT_PREFIX=/tmp/dwportal-root \
DWPORTAL_SKIP_SYSTEMD=1 \
script/deploy/manage.sh install --package dist/<package>.tar.gz
```

该模式把系统路径重定向到临时目录，并跳过 systemd，供自动测试使用。
