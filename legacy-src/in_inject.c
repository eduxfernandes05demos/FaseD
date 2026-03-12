/*
in_inject.c -- Programmatic input injection driver for cloud/container builds.

The streaming gateway forwards browser keyboard/mouse events to these
functions.  The events are queued and dequeued in IN_Move() on the next
engine frame.

Build this file when HEADLESS=1 is defined (cmake -DHEADLESS=ON).
*/

#include "quakedef.h"

#include <string.h>

/* -----------------------------------------------------------------------
 * Event queues
 * --------------------------------------------------------------------- */
#define MAX_INJECT_EVENTS 64

typedef struct {
	int      key;
	qboolean down;
} key_event_t;

typedef struct {
	int dx;
	int dy;
	int buttons;
} mouse_event_t;

static key_event_t   key_queue[MAX_INJECT_EVENTS];
static int           key_queue_head = 0;
static int           key_queue_tail = 0;

static mouse_event_t mouse_queue[MAX_INJECT_EVENTS];
static int           mouse_queue_head = 0;
static int           mouse_queue_tail = 0;

/* Accumulated mouse delta for the current frame */
static int accumulated_dx = 0;
static int accumulated_dy = 0;
static int accumulated_buttons = 0;

/* -----------------------------------------------------------------------
 * Public injection API (called by the streaming gateway / IPC layer)
 * --------------------------------------------------------------------- */

/*
IN_InjectKeyEvent
Inject a key press or release.  key is a Quake K_* constant.
*/
void IN_InjectKeyEvent (int key, qboolean down)
{
	int next_head = (key_queue_head + 1) % MAX_INJECT_EVENTS;
	if (next_head == key_queue_tail)
		return; /* queue full – drop event */

	key_queue[key_queue_head].key  = key;
	key_queue[key_queue_head].down = down;
	key_queue_head = next_head;
}

/*
IN_InjectMouseEvent
Inject a relative mouse movement and button state.
dx/dy are signed pixel deltas; buttons is a bitmask (bit0=left, bit1=right,
bit2=middle).
*/
void IN_InjectMouseEvent (int dx, int dy, int buttons)
{
	int next_head = (mouse_queue_head + 1) % MAX_INJECT_EVENTS;
	if (next_head == mouse_queue_tail)
		return; /* queue full – drop event */

	mouse_queue[mouse_queue_head].dx      = dx;
	mouse_queue[mouse_queue_head].dy      = dy;
	mouse_queue[mouse_queue_head].buttons = buttons;
	mouse_queue_head = next_head;
}

/* -----------------------------------------------------------------------
 * IN interface (called by the engine each frame)
 * --------------------------------------------------------------------- */

void IN_Init (void)
{
	key_queue_head = key_queue_tail = 0;
	mouse_queue_head = mouse_queue_tail = 0;
}

void IN_Shutdown (void)
{
}

void IN_Commands (void)
{
	/* Dequeue key events and feed them into the Quake key subsystem. */
	while (key_queue_tail != key_queue_head)
	{
		key_event_t *ev = &key_queue[key_queue_tail];
		Key_Event(ev->key, ev->down);
		key_queue_tail = (key_queue_tail + 1) % MAX_INJECT_EVENTS;
	}
}

void IN_Move (usercmd_t *cmd)
{
	/* Dequeue mouse events and accumulate deltas for this frame. */
	accumulated_dx      = 0;
	accumulated_dy      = 0;
	accumulated_buttons = 0;

	while (mouse_queue_tail != mouse_queue_head)
	{
		mouse_event_t *ev = &mouse_queue[mouse_queue_tail];
		accumulated_dx      += ev->dx;
		accumulated_dy      += ev->dy;
		accumulated_buttons  = ev->buttons; /* last state wins */
		mouse_queue_tail = (mouse_queue_tail + 1) % MAX_INJECT_EVENTS;
	}

	/* Apply mouse look: horizontal → yaw, vertical → pitch */
	if (accumulated_dx || accumulated_dy)
	{
		extern cvar_t sensitivity;
		cl.viewangles[YAW]   -= sensitivity.value * (float)accumulated_dx;
		cl.viewangles[PITCH] += sensitivity.value * (float)accumulated_dy;

		if (cl.viewangles[PITCH] > 80.0f)
			cl.viewangles[PITCH] = 80.0f;
		if (cl.viewangles[PITCH] < -70.0f)
			cl.viewangles[PITCH] = -70.0f;
	}
}

void IN_ClearStates (void)
{
	accumulated_dx      = 0;
	accumulated_dy      = 0;
	accumulated_buttons = 0;
}
