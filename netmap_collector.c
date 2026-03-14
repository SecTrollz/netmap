/*
 * netmap_collect.c – Linux network state collector
 * Uses netlink and /proc only. No inline assembly, pure C.
 * Compile: gcc -Wall -O2 -o netmap_collect netmap_collect.c
 */

#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <errno.h>
#include <sys/socket.h>
#include <linux/netlink.h>
#include <linux/rtnetlink.h>
#include <net/if.h>
#include <arpa/inet.h>
#include <sys/types.h>
#include <sys/stat.h>
#include <fcntl.h>

/* Dump netlink interfaces */
static void dump_links(void) {
    struct {
        struct nlmsghdr nlh;
        struct rtgenmsg g;
    } req;
    int fd = socket(AF_NETLINK, SOCK_RAW, NETLINK_ROUTE);
    if (fd < 0) {
        fprintf(stderr, "ERROR: netlink socket: %s\n", strerror(errno));
        return;
    }
    memset(&req, 0, sizeof(req));
    req.nlh.nlmsg_len = NLMSG_LENGTH(sizeof(struct rtgenmsg));
    req.nlh.nlmsg_type = RTM_GETLINK;
    req.nlh.nlmsg_flags = NLM_F_REQUEST | NLM_F_DUMP;
    req.nlh.nlmsg_seq = 1;
    req.nlh.nlmsg_pid = getpid();
    req.g.rtgen_family = AF_PACKET;

    if (send(fd, &req, req.nlh.nlmsg_len, 0) < 0) {
        fprintf(stderr, "ERROR: netlink send: %s\n", strerror(errno));
        close(fd);
        return;
    }
    char buf[16384];
    int len;
    while ((len = recv(fd, buf, sizeof(buf), 0)) > 0) {
        struct nlmsghdr *nh;
        for (nh = (struct nlmsghdr*)buf; NLMSG_OK(nh, len); nh = NLMSG_NEXT(nh, len)) {
            if (nh->nlmsg_type == NLMSG_DONE) break;
            if (nh->nlmsg_type == NLMSG_ERROR) {
                fprintf(stderr, "ERROR: netlink error\n");
                close(fd);
                return;
            }
            struct ifinfomsg *ifi = NLMSG_DATA(nh);
            char name[IFNAMSIZ];
            if_indextoname(ifi->ifi_index, name);
            printf("LINK: index=%d name=%s flags=0x%x type=%d\n",
                   ifi->ifi_index, name, ifi->ifi_flags, ifi->ifi_type);
        }
    }
    close(fd);
}

/* Dump addresses (similar pattern, omitted for brevity – full code available on request) */
/* Dump routes */
/* Read /proc/net/tcp, udp, unix */
/* Read /etc/resolv.conf */

int main(void) {
    dump_links();
    // ... other dumps
    return 0;
}
