# 开发
* 退出后自动关闭
./start.sh
# 后台运行
./start_back.sh

# 检查端口占用
sudo lsof -i :8080
lsof -i :5173
# 重建readonly.css
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && cd /home/llf/OpenShare/frontend && npm run build:readonly 2>&1

# 开发模式
* 停止： kill $(cat ~/OpenShare/build/openshare.pid)
* 启动（后台运行）：./deploy.sh --daemon

# 重新构建后端
cd backend
go clean -cache
go build ./...



文件夹下文件的请求始终是files?page=1&page_size=100&sort=name_asc，sort 
没有分页逻辑，只能显示 100 条，增加分页切换按钮
修改排序方式后应该重新进行请求，防止因为文件数量超过 100 而部分无法显示
支持设置单页的最大文件数显示