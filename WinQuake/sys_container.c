/*
sys_container.c -- System layer for headless container / cloud builds.

Replaces sys_linux.c when HEADLESS=1 is defined.

Key differences from the desktop sys_linux.c:
  - Console output is emitted as structured JSON to stdout so that log
    aggregation (Azure Monitor / Log Analytics) can parse it.
  - SIGTERM triggers a clean shutdown via Host_Shutdown().
  - Configuration is read from environment variables:
      QUAKE_BASEDIR  - path to the Quake data directory (default: /game)
      QUAKE_MAP      - map to start on launch (default: e1m1)
      QUAKE_SKILL    - skill level 0-3 (default: 1)
      QUAKE_MEM_MB   - memory in MB for the hunk (default: 32)
  - A minimal HTTP health endpoint at /healthz is provided via a small
    background thread so the Azure Container App can probe liveness.
*/

#include <unistd.h>
#include <signal.h>
#include <stdlib.h>
#include <limits.h>
#include <sys/time.h>
#include <sys/types.h>
#include <sys/stat.h>
#include <fcntl.h>
#include <stdarg.h>
#include <stdio.h>
#include <string.h>
#include <errno.h>
#include <netinet/in.h>
#include <arpa/inet.h>
#include <pthread.h>
#include <time.h>
#include <execinfo.h>

#include "quakedef.h"

extern void FrameServer_Init (void);
extern void FrameServer_Shutdown (void);

/* -----------------------------------------------------------------------
 * Global state
 * --------------------------------------------------------------------- */
qboolean isDedicated = false;

static volatile sig_atomic_t host_shutdown_requested = 0;
static volatile sig_atomic_t engine_running = 0;   /* set to 1 once Host_Frame loop starts */

/* -----------------------------------------------------------------------
 * Structured JSON logging
 * --------------------------------------------------------------------- */
static void json_log (const char *level, const char *msg)
{
	time_t now;
	struct tm tm_info;
	char ts[32];

	time(&now);
	gmtime_r(&now, &tm_info);
	strftime(ts, sizeof(ts), "%Y-%m-%dT%H:%M:%SZ", &tm_info);

	/* Escape any embedded quotes in msg (simple approach) */
	printf("{\"time\":\"%s\",\"level\":\"%s\",\"msg\":\"", ts, level);
	for (const char *p = msg; *p; p++) {
		if (*p == '"')  printf("\\\"");
		else if (*p == '\\') printf("\\\\");
		else if (*p == '\n') printf("\\n");
		else            putchar(*p);
	}
	printf("\"}\n");
	fflush(stdout);
}

/* -----------------------------------------------------------------------
 * Signal handlers
 * --------------------------------------------------------------------- */
static void handle_sigsegv (int sig)
{
	void *bt[32];
	int n;
	(void)sig;
	n = backtrace(bt, 32);
	fprintf(stderr, "SIGSEGV caught, backtrace:\n");
	backtrace_symbols_fd(bt, n, STDERR_FILENO);
	_exit(139);
}

static void handle_sigterm (int sig)
{
	(void)sig;
	host_shutdown_requested = 1;
}

/* -----------------------------------------------------------------------
 * Minimal /healthz HTTP server (background thread)
 * --------------------------------------------------------------------- */
#define HEALTH_PORT 8080

static void *healthz_thread (void *arg)
{
	int server_fd, client_fd;
	struct sockaddr_in addr;
	socklen_t addrlen = sizeof(addr);
	int opt = 1;
	char buf[256];
	const char *ok_resp =
		"HTTP/1.1 200 OK\r\n"
		"Content-Type: text/plain\r\n"
		"Content-Length: 2\r\n"
		"Connection: close\r\n"
		"\r\nOK";
	const char *busy_resp =
		"HTTP/1.1 503 Service Unavailable\r\n"
		"Content-Type: text/plain\r\n"
		"Content-Length: 12\r\n"
		"Connection: close\r\n"
		"\r\nInitializing";

	(void)arg;

	server_fd = socket(AF_INET, SOCK_STREAM, 0);
	if (server_fd < 0) return NULL;

	setsockopt(server_fd, SOL_SOCKET, SO_REUSEADDR, &opt, sizeof(opt));

	memset(&addr, 0, sizeof(addr));
	addr.sin_family      = AF_INET;
	addr.sin_addr.s_addr = INADDR_ANY;
	addr.sin_port        = htons(HEALTH_PORT);

	if (bind(server_fd, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
		close(server_fd);
		return NULL;
	}

	listen(server_fd, 4);

	while (!host_shutdown_requested) {
		fd_set fds;
		struct timeval tv = { 1, 0 };

		FD_ZERO(&fds);
		FD_SET(server_fd, &fds);

		if (select(server_fd + 1, &fds, NULL, NULL, &tv) <= 0)
			continue;

		client_fd = accept(server_fd, (struct sockaddr *)&addr, &addrlen);
		if (client_fd < 0) continue;

		/* Drain the request */
		recv(client_fd, buf, sizeof(buf) - 1, 0);

		if (engine_running)
			send(client_fd, ok_resp, strlen(ok_resp), 0);
		else
			send(client_fd, busy_resp, strlen(busy_resp), 0);

		close(client_fd);
	}

	close(server_fd);
	return NULL;
}

/* -----------------------------------------------------------------------
 * Sys interface
 * --------------------------------------------------------------------- */

void Sys_Printf (char *fmt, ...)
{
	va_list argptr;
	char text[1024];

	va_start(argptr, fmt);
	vsnprintf(text, sizeof(text), fmt, argptr);
	va_end(argptr);

	json_log("info", text);
}

void Sys_Error (char *error, ...)
{
	va_list argptr;
	char string[1024];

	va_start(argptr, error);
	vsnprintf(string, sizeof(string), error, argptr);
	va_end(argptr);

	json_log("error", string);
	Host_Shutdown();
	exit(1);
}

void Sys_Quit (void)
{
	json_log("info", "Sys_Quit: clean exit");
	Host_Shutdown();
	exit(0);
}

double Sys_FloatTime (void)
{
	struct timeval tp;
	struct timezone tzp;
	static int secbase = 0;

	gettimeofday(&tp, &tzp);

	if (!secbase) {
		secbase = tp.tv_sec;
		return tp.tv_usec / 1000000.0;
	}

	return (tp.tv_sec - secbase) + tp.tv_usec / 1000000.0;
}

char *Sys_ConsoleInput (void)
{
	return NULL;
}

void Sys_Sleep (void)
{
	usleep(1000);
}

void Sys_SendKeyEvents (void)
{
}

void Sys_HighFPPrecision (void)
{
}

void Sys_LowFPPrecision (void)
{
}

void Sys_Init (void)
{
}

void Sys_mkdir (char *path)
{
	mkdir(path, 0777);
}

int Sys_FileTime (char *path)
{
	struct stat buf;
	if (stat(path, &buf) == -1)
		return -1;
	return buf.st_mtime;
}

int Sys_FileOpenRead (char *path, int *handle)
{
	int h;
	struct stat fileinfo;

	h = open(path, O_RDONLY, 0666);
	*handle = h;
	if (h == -1)
		return -1;
	if (fstat(h, &fileinfo) == -1)
		Sys_Error("Error fstating %s", path);
	return fileinfo.st_size;
}

int Sys_FileOpenWrite (char *path)
{
	int handle;

	umask(0);
	handle = open(path, O_RDWR | O_CREAT | O_TRUNC, 0666);
	if (handle == -1)
		Sys_Error("Error opening %s: %s", path, strerror(errno));
	return handle;
}

int Sys_FileWrite (int handle, void *src, int count)
{
	return write(handle, src, count);
}

void Sys_FileClose (int handle)
{
	close(handle);
}

void Sys_FileSeek (int handle, int position)
{
	lseek(handle, position, SEEK_SET);
}

int Sys_FileRead (int handle, void *dest, int count)
{
	return read(handle, dest, count);
}

void Sys_MakeCodeWriteable (unsigned long startaddr, unsigned long length)
{
	/* Not needed for non-ASM builds */
	(void)startaddr;
	(void)length;
}

void Sys_DebugLog (char *file, char *fmt, ...)
{
	(void)file;
	(void)fmt;
}

/* -----------------------------------------------------------------------
 * main()
 * --------------------------------------------------------------------- */
int main (int c, char **v)
{
	static quakeparms_t parms;
	const char *basedir_env, *map_env, *skill_env, *mem_env;
	int mem_mb;
	char startmap_cmd[128];
	pthread_t health_tid;
	double oldtime, newtime, time;

	signal(SIGTERM, handle_sigterm);
	signal(SIGINT,  handle_sigterm);
	signal(SIGPIPE, SIG_IGN);
	signal(SIGFPE,  SIG_IGN);
	signal(SIGSEGV, handle_sigsegv);

	/* Read configuration from environment */
	basedir_env = getenv("QUAKE_BASEDIR");
	map_env     = getenv("QUAKE_MAP");
	skill_env   = getenv("QUAKE_SKILL");
	mem_env     = getenv("QUAKE_MEM_MB");

	if (!basedir_env) basedir_env = "/game";
	if (!map_env)     map_env     = "e1m1";
	if (!skill_env)   skill_env   = "1";

	mem_mb = mem_env ? atoi(mem_env) : 32;
	if (mem_mb < 8)  mem_mb = 8;
	if (mem_mb > 512) mem_mb = 512;

	memset(&parms, 0, sizeof(parms));
	COM_InitArgv(c, v);
	parms.argc    = com_argc;
	parms.argv    = com_argv;
	parms.memsize = mem_mb * 1024 * 1024;
	parms.membase = malloc(parms.memsize);
	if (!parms.membase) {
		fprintf(stderr, "sys_container: out of memory\n");
		exit(1);
	}
	parms.basedir = basedir_env;

	/* Start health endpoint before engine init so probes succeed earlier */
	pthread_create(&health_tid, NULL, healthz_thread, NULL);

	/* Start frame server for streaming gateway IPC */
	FrameServer_Init();

	json_log("info", "Host_Init starting");
	Host_Init(&parms);
	json_log("info", "Host_Init complete");

	/* Queue the start map command */
	snprintf(startmap_cmd, sizeof(startmap_cmd),
	         "skill %s; map %s\n", skill_env, map_env);
	Cbuf_AddText(startmap_cmd);

	engine_running = 1;
	json_log("info", "Engine frame loop started");

	oldtime = Sys_FloatTime() - 0.1;
	while (!host_shutdown_requested) {
		newtime = Sys_FloatTime();
		time    = newtime - oldtime;

		if (time < 0.0) time = 0.0;
		if (time > 0.2) time = 0.2;   /* clamp to 5 fps minimum */

		oldtime += time;
		Host_Frame(time);
	}

	engine_running = 0;
	json_log("info", "SIGTERM received – shutting down");
	FrameServer_Shutdown();
	Host_Shutdown();

	pthread_join(health_tid, NULL);
	return 0;
}
