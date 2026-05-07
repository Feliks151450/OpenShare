# 开发
* 退出后自动关闭
nohup ./start.sh &
# 后台运行
nohup ./start_back.sh &

# 重建readonly.css
tailwindcss -c tailwind.config.ts -i ./standalone-readonly/readonly.input.css -o ./standalone-readonly/readonly.css --minify