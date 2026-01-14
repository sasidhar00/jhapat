FROM alpine:latest
RUN apk add --no-cache ca-certificates

WORKDIR /root/

# Copy the file you just built on your computer
COPY server .
# Copy your HTML/CSS folder
COPY public ./public

# Make it executable and run it
RUN chmod +x ./server
EXPOSE 8000
CMD ["./server"]
