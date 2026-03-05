/*
snd_capture.c -- Headless audio driver for cloud/container builds.

Implements the SNDDMA interface backed by an in-memory ring buffer.
Mixed PCM audio can be retrieved by the streaming gateway via
SND_CaptureAudio().

Build this file when HEADLESS=1 is defined (cmake -DHEADLESS=ON).
*/

#include "quakedef.h"
#include "sound.h"

#include <stdlib.h>
#include <string.h>

/* -----------------------------------------------------------------------
 * Ring buffer configuration
 * The buffer must be large enough to hold several video frames worth of
 * audio.  At 11025 Hz, 16-bit mono, 1 second = 22050 bytes.
 * We allocate 2 seconds (44100 bytes) rounded up to a power of two.
 * --------------------------------------------------------------------- */
#define CAPTURE_SPEED       11025
#define CAPTURE_SAMPLEBITS  16
#define CAPTURE_CHANNELS    1
#define CAPTURE_BUFSIZE     (1 << 16)   /* 65536 bytes (~3 s at 11 kHz mono 16-bit) */

static unsigned char capture_buf[CAPTURE_BUFSIZE];

/* Monotonically advancing DMA position in samples (never wraps) */
static volatile int dma_position = 0;

/* -----------------------------------------------------------------------
 * SNDDMA interface
 * --------------------------------------------------------------------- */

qboolean SNDDMA_Init (void)
{
	memset(capture_buf, 0, sizeof(capture_buf));
	dma_position = 0;

	shm = &sn;
	shm->splitbuffer      = 0;
	shm->channels         = CAPTURE_CHANNELS;
	shm->samplebits       = CAPTURE_SAMPLEBITS;
	shm->speed            = CAPTURE_SPEED;
	shm->buffer           = capture_buf;
	shm->samples          = CAPTURE_BUFSIZE / (CAPTURE_SAMPLEBITS / 8);
	shm->samplepos        = 0;
	shm->submission_chunk = 1;
	shm->soundalive       = true;
	shm->gamealive        = true;

	Sys_Printf("SNDDMA_Init: capture ring buffer %d Hz %d-bit %s\n",
	           CAPTURE_SPEED, CAPTURE_SAMPLEBITS,
	           CAPTURE_CHANNELS == 1 ? "mono" : "stereo");
	return true;
}

int SNDDMA_GetDMAPos (void)
{
	/* Advance position by one submission chunk per call to simulate DMA
	 * progress.  The snd_dma.c mixer uses this to determine how much new
	 * data it should mix. */
	dma_position += shm->submission_chunk;
	shm->samplepos = dma_position % shm->samples;
	return shm->samplepos;
}

void SNDDMA_Shutdown (void)
{
	if (shm)
		shm->soundalive = false;
}

void SNDDMA_Submit (void)
{
	/* Nothing to do – buffer is already in memory. */
}

/* -----------------------------------------------------------------------
 * SND_CaptureAudio
 *
 * Retrieve a pointer to the raw PCM ring buffer so the streaming gateway
 * can encode it.
 *
 * *pcm     - pointer to the ring buffer (caller must NOT free or modify it)
 * *samples - total capacity of the ring buffer in mono samples
 * *rate    - sample rate in Hz
 *
 * The current write position is shm->samplepos.  The caller should read
 * backward from samplepos to consume freshly mixed samples.
 * --------------------------------------------------------------------- */
void SND_CaptureAudio (short **pcm, int *samples, int *rate)
{
	*pcm     = (short *)capture_buf;
	*samples = CAPTURE_BUFSIZE / sizeof(short);
	*rate    = CAPTURE_SPEED;
}
