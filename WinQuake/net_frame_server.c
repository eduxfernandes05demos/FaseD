/*
net_frame_server.c -- TCP frame server for streaming gateway IPC.

Exposes video frames, audio samples, and input injection over a simple
binary protocol on a configurable TCP port (default 9000).

The streaming gateway connects as a TCP client and issues single-byte
commands to request frames, audio, or inject input events.

Protocol (all integers little-endian):

  'F' (0x46) — Get Frame
    Response: [width:4B][height:4B][jpeg_len:4B][jpeg_data:*]

  'A' (0x41) — Get Audio
    Response: [samples:4B][rate:4B][pcm_data:*]

  'K' (0x4B) — Inject Key Event
    Client sends after command: [key:4B][down:1B]
    Response: [ok:1B] (0x01)

  'M' (0x4D) — Inject Mouse Event
    Client sends after command: [dx:4B][dy:4B][buttons:4B]
    Response: [ok:1B] (0x01)

Build this file when HEADLESS=1 is defined (cmake -DHEADLESS=ON).
*/

#include "quakedef.h"

#include <stdlib.h>
#include <string.h>
#include <inttypes.h>
#include <unistd.h>
#include <errno.h>
#include <netinet/in.h>
#include <netinet/tcp.h>
#include <sys/socket.h>
#include <sys/select.h>
#include <pthread.h>

/* stb_image_write for JPEG encoding — implementation in this TU */
#define STB_IMAGE_WRITE_IMPLEMENTATION
#define STBI_WRITE_NO_STDIO
#include "stb_image_write.h"

/* -----------------------------------------------------------------------
 * External APIs from sibling drivers
 * --------------------------------------------------------------------- */
extern int  VID_CaptureFrame (byte **rgba, int *width, int *height);
extern void SND_CaptureAudio (short **pcm, int *samples, int *rate);
extern void IN_InjectKeyEvent (int key, qboolean down);
extern void IN_InjectMouseEvent (int dx, int dy, int buttons);

/* Mutex for frame capture (shared with VID_Update in vid_headless.c) */
extern pthread_mutex_t frame_mutex;

/* -----------------------------------------------------------------------
 * Configuration
 * --------------------------------------------------------------------- */
#define DEFAULT_FRAME_SERVER_PORT 9000

static volatile int frame_server_running = 0;

/* -----------------------------------------------------------------------
 * JPEG write callback — accumulates bytes into a growable buffer
 * --------------------------------------------------------------------- */
typedef struct {
	unsigned char *data;
	int            size;
	int            capacity;
} jpeg_buf_t;

static void jpeg_write_func (void *context, void *data, int size)
{
	jpeg_buf_t *buf = (jpeg_buf_t *)context;
	int new_size = buf->size + size;

	if (new_size > buf->capacity)
	{
		int new_cap = buf->capacity * 2;
		unsigned char *tmp;
		if (new_cap < new_size)
			new_cap = new_size;
		tmp = (unsigned char *)realloc(buf->data, new_cap);
		if (!tmp) return;  /* drop bytes on OOM */
		buf->data     = tmp;
		buf->capacity = new_cap;
	}

	memcpy(buf->data + buf->size, data, size);
	buf->size = new_size;
}

/* -----------------------------------------------------------------------
 * Reliable send — loops until all bytes are written or error
 * --------------------------------------------------------------------- */
static int send_all (int fd, const void *buf, int len)
{
	const char *p = (const char *)buf;
	int remaining = len;

	while (remaining > 0)
	{
		int n = send(fd, p, remaining, MSG_NOSIGNAL);
		if (n <= 0) return -1;
		p         += n;
		remaining -= n;
	}
	return len;
}

/* -----------------------------------------------------------------------
 * Reliable recv — loops until all bytes are read or error
 * --------------------------------------------------------------------- */
static int recv_all (int fd, void *buf, int len)
{
	char *p = (char *)buf;
	int remaining = len;

	while (remaining > 0)
	{
		int n = recv(fd, p, remaining, 0);
		if (n <= 0) return -1;
		p         += n;
		remaining -= n;
	}
	return len;
}

/* -----------------------------------------------------------------------
 * Command: 'F' — Capture frame and send as JPEG
 * --------------------------------------------------------------------- */
static int handle_frame_request (int client_fd, jpeg_buf_t *jbuf)
{
	byte *rgba = NULL;
	int   w = 0, h = 0;
	int   header[3]; /* width, height, jpeg_len (LE) */

	/* Capture the frame under lock */
	pthread_mutex_lock(&frame_mutex);
	VID_CaptureFrame(&rgba, &w, &h);
	if (!rgba || w == 0 || h == 0)
	{
		pthread_mutex_unlock(&frame_mutex);
		/* Send a zero-size frame */
		memset(header, 0, sizeof(header));
		return send_all(client_fd, header, sizeof(header));
	}

	/* Encode to JPEG */
	jbuf->size = 0;
	stbi_write_jpg_to_func(jpeg_write_func, jbuf, w, h, 4, rgba, 70);
	pthread_mutex_unlock(&frame_mutex);

	if (jbuf->size == 0)
	{
		memset(header, 0, sizeof(header));
		return send_all(client_fd, header, sizeof(header));
	}

	header[0] = w;
	header[1] = h;
	header[2] = jbuf->size;

	if (send_all(client_fd, header, sizeof(header)) < 0) return -1;
	if (send_all(client_fd, jbuf->data, jbuf->size) < 0) return -1;
	return 0;
}

/* -----------------------------------------------------------------------
 * Command: 'A' — Send current audio buffer
 * --------------------------------------------------------------------- */
static int handle_audio_request (int client_fd)
{
	short *pcm = NULL;
	int    samples = 0, rate = 0;
	int    header[2]; /* samples, rate (LE) */
	int    pcm_bytes;

	SND_CaptureAudio(&pcm, &samples, &rate);

	if (!pcm || samples == 0)
	{
		memset(header, 0, sizeof(header));
		return send_all(client_fd, header, sizeof(header));
	}

	header[0] = samples;
	header[1] = rate;
	pcm_bytes = samples * (int)sizeof(short);

	if (send_all(client_fd, header, sizeof(header)) < 0) return -1;
	if (send_all(client_fd, pcm, pcm_bytes) < 0)         return -1;
	return 0;
}

/* -----------------------------------------------------------------------
 * Command: 'K' — Inject a key event
 * --------------------------------------------------------------------- */
static int handle_key_inject (int client_fd)
{
	int  key;
	byte down;
	byte ok = 0x01;

	if (recv_all(client_fd, &key, 4) < 0) return -1;
	if (recv_all(client_fd, &down, 1) < 0) return -1;

	IN_InjectKeyEvent(key, down ? true : false);

	return send_all(client_fd, &ok, 1);
}

/* -----------------------------------------------------------------------
 * Command: 'M' — Inject a mouse event
 * --------------------------------------------------------------------- */
static int handle_mouse_inject (int client_fd)
{
	int  payload[3]; /* dx, dy, buttons */
	byte ok = 0x01;

	if (recv_all(client_fd, payload, sizeof(payload)) < 0) return -1;

	IN_InjectMouseEvent(payload[0], payload[1], payload[2]);

	return send_all(client_fd, &ok, 1);
}

/* -----------------------------------------------------------------------
 * Client session — runs in the server thread, handles one client at a time
 * --------------------------------------------------------------------- */
static void handle_client (int client_fd, jpeg_buf_t *jbuf)
{
	int flag = 1;
	setsockopt(client_fd, IPPROTO_TCP, TCP_NODELAY, &flag, sizeof(flag));

	while (frame_server_running)
	{
		byte cmd;
		int  n;

		n = recv(client_fd, &cmd, 1, 0);
		if (n <= 0) break; /* client disconnected or error */

		switch (cmd)
		{
		case 'F':
			if (handle_frame_request(client_fd, jbuf) < 0) return;
			break;
		case 'A':
			if (handle_audio_request(client_fd) < 0) return;
			break;
		case 'K':
			if (handle_key_inject(client_fd) < 0) return;
			break;
		case 'M':
			if (handle_mouse_inject(client_fd) < 0) return;
			break;
		default:
			/* Unknown command — disconnect */
			return;
		}
	}
}

/* -----------------------------------------------------------------------
 * Frame server thread — listens, accepts one client at a time
 * --------------------------------------------------------------------- */
static void *frame_server_thread (void *arg)
{
	int server_fd, client_fd;
	struct sockaddr_in addr;
	socklen_t addrlen = sizeof(addr);
	int opt = 1;
	int port;
	const char *port_env;
	jpeg_buf_t jbuf;

	(void)arg;

	/* Determine port */
	port_env = getenv("FRAME_SERVER_PORT");
	port = port_env ? atoi(port_env) : DEFAULT_FRAME_SERVER_PORT;
	if (port <= 0 || port > 65535)
		port = DEFAULT_FRAME_SERVER_PORT;

	/* Pre-allocate JPEG buffer (will grow if needed) */
	jbuf.capacity = 256 * 1024; /* 256 KB initial — enough for 640×480 JPEG */
	jbuf.data = (unsigned char *)malloc(jbuf.capacity);
	jbuf.size = 0;
	if (!jbuf.data)
	{
		Sys_Printf("FrameServer: out of memory for JPEG buffer\n");
		return NULL;
	}

	server_fd = socket(AF_INET, SOCK_STREAM, 0);
	if (server_fd < 0)
	{
		Sys_Printf("FrameServer: socket() failed: %s\n", strerror(errno));
		free(jbuf.data);
		return NULL;
	}

	setsockopt(server_fd, SOL_SOCKET, SO_REUSEADDR, &opt, sizeof(opt));

	memset(&addr, 0, sizeof(addr));
	addr.sin_family      = AF_INET;
	addr.sin_addr.s_addr = htonl(INADDR_LOOPBACK); /* localhost only */
	addr.sin_port        = htons((unsigned short)port);

	if (bind(server_fd, (struct sockaddr *)&addr, sizeof(addr)) < 0)
	{
		Sys_Printf("FrameServer: bind(:%" PRId32 ") failed: %s\n",
		           port, strerror(errno));
		close(server_fd);
		free(jbuf.data);
		return NULL;
	}

	listen(server_fd, 1);

	frame_server_running = 1;
	Sys_Printf("FrameServer: listening on 127.0.0.1:%d\n", port);

	while (frame_server_running)
	{
		fd_set fds;
		struct timeval tv = { 1, 0 };

		FD_ZERO(&fds);
		FD_SET(server_fd, &fds);

		if (select(server_fd + 1, &fds, NULL, NULL, &tv) <= 0)
			continue;

		client_fd = accept(server_fd, (struct sockaddr *)&addr, &addrlen);
		if (client_fd < 0)
			continue;

		Sys_Printf("FrameServer: client connected\n");
		handle_client(client_fd, &jbuf);
		close(client_fd);
		Sys_Printf("FrameServer: client disconnected\n");
	}

	close(server_fd);
	free(jbuf.data);
	return NULL;
}

/* -----------------------------------------------------------------------
 * Public API
 * --------------------------------------------------------------------- */
static pthread_t fs_thread;

void FrameServer_Init (void)
{
	pthread_create(&fs_thread, NULL, frame_server_thread, NULL);
}

void FrameServer_Shutdown (void)
{
	frame_server_running = 0;
	pthread_join(fs_thread, NULL);
}
