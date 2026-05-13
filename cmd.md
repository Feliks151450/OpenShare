# 开发
* 退出后自动关闭
./start.sh
# 后台运行
./start_back.sh

# 检查端口占用
sudo lsof -i :8080
# 重建readonly.css
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && cd /home/llf/OpenShare/frontend && npm run build:readonly 2>&1

# 开发模式
* 停止： kill $(cat /Users/linlifei/OpenShare/build/openshare.pid)
* 启动（后台运行）：./deploy.sh --daemon