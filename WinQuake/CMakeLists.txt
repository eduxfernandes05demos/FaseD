cmake_minimum_required(VERSION 3.16)
project(QuakeWorker C)

set(CMAKE_C_STANDARD 99)

# Build options
option(HEADLESS "Build headless cloud-worker variant (no display, no audio hardware)" OFF)
option(NOASM    "Disable x86 assembly and use portable C replacements"                ON)

# Common compile definitions
add_compile_definitions(
    Dstricmp=strcasecmp
    _GNU_SOURCE
)

if(NOASM)
    add_compile_definitions(id386=0)
endif()

# Compiler flags
add_compile_options(
    -Wall
    -Wno-unused-result
    -fno-strict-aliasing
    -fcommon
)

# -------------------------------------------------------------------------
# Source lists
# -------------------------------------------------------------------------

# Core engine sources (platform-independent)
set(QUAKE_CORE_SRCS
    cl_demo.c
    cl_input.c
    cl_main.c
    cl_parse.c
    cl_tent.c
    chase.c
    cmd.c
    common.c
    console.c
    crc.c
    cvar.c
    draw.c
    d_edge.c
    d_fill.c
    d_init.c
    d_modech.c
    d_part.c
    d_polyse.c
    d_scan.c
    d_sky.c
    d_sprite.c
    d_surf.c
    d_vars.c
    d_zpoint.c
    host.c
    host_cmd.c
    keys.c
    menu.c
    mathlib.c
    model.c
    net_dgrm.c
    net_loop.c
    net_main.c
    net_vcr.c
    net_udp.c
    net_bsd.c
    nonintel.c
    pr_cmds.c
    pr_edict.c
    pr_exec.c
    r_aclip.c
    r_alias.c
    r_bsp.c
    r_light.c
    r_draw.c
    r_efrag.c
    r_edge.c
    r_misc.c
    r_main.c
    r_sky.c
    r_sprite.c
    r_surf.c
    r_part.c
    r_vars.c
    screen.c
    sbar.c
    sv_main.c
    sv_phys.c
    sv_move.c
    sv_user.c
    zone.c
    view.c
    wad.c
    world.c
    snd_mem.c
    snd_mix.c
)

# Platform-specific sources selected at configure time
if(HEADLESS)
    # Headless / cloud-worker drivers
    list(APPEND QUAKE_CORE_SRCS
        vid_headless.c
        snd_capture.c
        snd_dma.c
        in_inject.c
        net_frame_server.c
        sys_container.c
        cd_null.c
    )
    add_compile_definitions(HEADLESS=1)
else()
    # Standard Linux desktop drivers
    list(APPEND QUAKE_CORE_SRCS
        vid_svgalib.c
        snd_dma.c
        snd_linux.c
        in_null.c
        sys_linux.c
        cd_linux.c
    )
endif()

# -------------------------------------------------------------------------
# Target
# -------------------------------------------------------------------------

set(TARGET_NAME quake-worker)

add_executable(${TARGET_NAME} ${QUAKE_CORE_SRCS})

target_include_directories(${TARGET_NAME} PRIVATE ${CMAKE_CURRENT_SOURCE_DIR})

# Link math library (always needed)
target_link_libraries(${TARGET_NAME} m)

if(NOT HEADLESS)
    # Desktop build needs SVGA and sound libraries
    target_link_libraries(${TARGET_NAME} vga)
else()
    # Container build needs pthreads for the health endpoint thread
    find_package(Threads REQUIRED)
    target_link_libraries(${TARGET_NAME} Threads::Threads)
endif()

# -------------------------------------------------------------------------
# Optional: AddressSanitizer support
# -------------------------------------------------------------------------
option(ASAN "Enable AddressSanitizer" OFF)
if(ASAN)
    target_compile_options(${TARGET_NAME} PRIVATE -fsanitize=address -fno-omit-frame-pointer)
    target_link_options(${TARGET_NAME} PRIVATE -fsanitize=address)
endif()

# -------------------------------------------------------------------------
# Install
# -------------------------------------------------------------------------
install(TARGETS ${TARGET_NAME} RUNTIME DESTINATION bin)
