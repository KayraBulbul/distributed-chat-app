FROM debian:stable-slim

COPY ./backend/server/server /bin/server

CMD ["/bin/server"]
