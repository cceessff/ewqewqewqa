server{
listen 80 default_server;
server_name domain.com  www.domain.com;
if($host !~ 'www\.') {
    rewrite ^/(.*) http://www.$host/$1 permanent;

 }
 resolver 114.114.114.114;
 access_log logs/domain.com.access.log;
 error_log logs/domain.com.error.log;

 location / {
    proxy_pass http://localhost:8899;
    proxy_set_header Accept-Encoding "";
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forward-For  $proxy_add_x_forwarded_for;
    proxy_set_header Host $host;
    proxy_cache off;
    proxy_set_header scheme $scheme;
 }
}