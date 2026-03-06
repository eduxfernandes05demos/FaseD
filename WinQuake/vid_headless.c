/*
vid_headless.c -- Headless software video driver for cloud/container builds.

Renders to an in-memory framebuffer.  Frames can be retrieved by the
streaming gateway via VID_CaptureFrame().

Build this file when HEADLESS=1 is defined (cmake -DHEADLESS=ON).
*/

#include "quakedef.h"
#include "d_local.h"

#include <stdlib.h>
#include <string.h>
#include <pthread.h>

/* Mutex shared with net_frame_server.c to protect framebuffer access */
pthread_mutex_t frame_mutex = PTHREAD_MUTEX_INITIALIZER;

/* -----------------------------------------------------------------------
 * Resolution (can be overridden by env var QUAKE_WIDTH / QUAKE_HEIGHT)
 * --------------------------------------------------------------------- */
#define DEFAULT_WIDTH  320
#define DEFAULT_HEIGHT 200

/* Internal buffers */
static int vid_width  = DEFAULT_WIDTH;
static int vid_height = DEFAULT_HEIGHT;

static byte	*vid_buffer   = NULL;
static short	*zbuffer_mem  = NULL;
static byte	*surfcache_mem = NULL;

/* D_SurfaceCacheForRes computes the correct size at runtime.
 * We keep this as a minimum fallback only. */
#define SURFCACHE_SIZE_MIN (512 * 1024)

/* RGBA capture buffer (exported) */
static byte	*rgba_buffer   = NULL;
static int	frame_captured  = 0;

unsigned short	d_8to16table[256];
unsigned int	d_8to24table[256];

/* -----------------------------------------------------------------------
 * VID_CaptureFrame
 *
 * Copy the current 8-bit indexed framebuffer to an RGBA byte array.
 * *rgba   - pointer to the internal RGBA buffer (caller must NOT free it)
 * *width  - frame width in pixels
 * *height - frame height in pixels
 *
 * Returns 1 if a new frame is available since the last call, 0 otherwise.
 * --------------------------------------------------------------------- */
int VID_CaptureFrame (byte **rgba, int *width, int *height)
{
	int i, npixels;
	byte *src;
	byte *dst;

	pthread_mutex_lock(&frame_mutex);

	if (!vid_buffer || !rgba_buffer)
	{
		pthread_mutex_unlock(&frame_mutex);
		*rgba   = NULL;
		*width  = 0;
		*height = 0;
		return 0;
	}

	npixels = vid_width * vid_height;
	src = vid_buffer;
	dst = rgba_buffer;

	/* Expand 8-bit palette indices to RGBA using d_8to24table.
	 * d_8to24table stores packed 0xRRGGBB in low 24 bits. */
	for (i = 0; i < npixels; i++, src++, dst += 4)
	{
		unsigned int colour = d_8to24table[*src];
		dst[0] = (colour >>  0) & 0xff; /* R */
		dst[1] = (colour >>  8) & 0xff; /* G */
		dst[2] = (colour >> 16) & 0xff; /* B */
		dst[3] = 255;                    /* A */
	}

	*rgba   = rgba_buffer;
	*width  = vid_width;
	*height = vid_height;
	frame_captured = 0;
	pthread_mutex_unlock(&frame_mutex);
	return 1;
}

/* -----------------------------------------------------------------------
 * VID interface
 * --------------------------------------------------------------------- */

void VID_SetPalette (unsigned char *palette)
{
	int i;
	unsigned char *p = palette;

	for (i = 0; i < 256; i++, p += 3)
	{
		d_8to24table[i] = ((unsigned int)p[0])
		                | ((unsigned int)p[1] << 8)
		                | ((unsigned int)p[2] << 16);
	}
}

void VID_ShiftPalette (unsigned char *palette)
{
	VID_SetPalette(palette);
}

void VID_Init (unsigned char *palette)
{
	const char *env_w, *env_h;
	int w = DEFAULT_WIDTH;
	int h = DEFAULT_HEIGHT;
	int surfcache_size;

	env_w = getenv("QUAKE_WIDTH");
	env_h = getenv("QUAKE_HEIGHT");
	if (env_w && atoi(env_w) > 0)
		w = atoi(env_w);
	if (env_h && atoi(env_h) > 0)
		h = atoi(env_h);

	vid_width  = w;
	vid_height = h;

	{
		int sc_size = D_SurfaceCacheForRes(w, h);
		if (sc_size < SURFCACHE_SIZE_MIN)
			sc_size = SURFCACHE_SIZE_MIN;
		surfcache_size = sc_size;
	}

	vid_buffer    = (byte *)malloc(w * h);
	zbuffer_mem   = (short *)malloc(w * h * sizeof(short));
	surfcache_mem = (byte *)malloc(surfcache_size);
	rgba_buffer   = (byte *)malloc(w * h * 4);

	if (!vid_buffer || !zbuffer_mem || !surfcache_mem || !rgba_buffer)
		Sys_Error("VID_Init: out of memory");

	memset(vid_buffer,    0, w * h);
	memset(zbuffer_mem,   0, w * h * sizeof(short));
	memset(surfcache_mem, 0, surfcache_size);
	memset(rgba_buffer,   0, w * h * 4);

	vid.maxwarpwidth  = vid.width    = vid.conwidth  = w;
	vid.maxwarpheight = vid.height   = vid.conheight = h;
	vid.aspect        = (float)w / (float)h;
	vid.numpages      = 1;
	vid.colormap      = host_colormap;
	vid.fullbright    = 256 - LittleLong(*((int *)vid.colormap + 2048));
	vid.buffer        = vid.conbuffer   = vid_buffer;
	vid.rowbytes      = vid.conrowbytes = w;

	d_pzbuffer = zbuffer_mem;
	D_InitCaches(surfcache_mem, surfcache_size);

	VID_SetPalette(palette);

	Sys_Printf("VID_Init: headless %dx%d framebuffer ready\n", w, h);
}

void VID_Shutdown (void)
{
	free(vid_buffer);
	free(zbuffer_mem);
	free(surfcache_mem);
	free(rgba_buffer);

	vid_buffer    = NULL;
	zbuffer_mem   = NULL;
	surfcache_mem = NULL;
	rgba_buffer   = NULL;
}

void VID_Update (vrect_t *rects)
{
	/* Mark that a new frame is ready for capture. */
	pthread_mutex_lock(&frame_mutex);
	frame_captured = 1;
	pthread_mutex_unlock(&frame_mutex);
}

void D_BeginDirectRect (int x, int y, byte *pbitmap, int width, int height)
{
}

void D_EndDirectRect (int x, int y, int width, int height)
{
}
