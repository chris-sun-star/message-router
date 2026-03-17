docker run -d \
  --name mysql \
  -p 3306:3306 \
  -e MYSQL_DATABASE=app \
  -e MYSQL_USER=app \
  -e MYSQL_PASSWORD=aaAA11__ \
  -e MYSQL_ROOT_PASSWORD=rootpass \
  mysql:8.0
