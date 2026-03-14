/*
 * netmap_collect.c – Linux network state collector
 * Uses netlink and /proc. Pure C.
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

/* Helper: read and parse netlink dumps */
static void dump_netlink(int type) {
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
    req.nlh.nlmsg_type = type;
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
            if (type == RTM_GETLINK) {
                struct ifinfomsg *ifi = NLMSG_DATA(nh);
                char name[IFNAMSIZ];
                if_indextoname(ifi->ifi_index, name);
                printf("LINK: index=%d name=%s flags=0x%x type=%d\n",
                       ifi->ifi_index, name, ifi->ifi_flags, ifi->ifi_type);
            } else if (type == RTM_GETADDR) {
                struct ifaddrmsg *ifa = NLMSG_DATA(nh);
                struct rtattr *rta = IFA_RTA(ifa);
                int rtl = IFA_PAYLOAD(nh);
                char addr[INET6_ADDRSTRLEN] = "";
                for (; RTA_OK(rta, rtl); rta = RTA_NEXT(rta, rtl)) {
                    if (rta->rta_type == IFA_LOCAL || rta->rta_type == IFA_ADDRESS) {
                        if (ifa->ifa_family == AF_INET)
                            inet_ntop(AF_INET, RTA_DATA(rta), addr, sizeof(addr));
                        else if (ifa->ifa_family == AF_INET6)
                            inet_ntop(AF_INET6, RTA_DATA(rta), addr, sizeof(addr));
                        else continue;
                        printf("ADDR: index=%d family=%d prefixlen=%d addr=%s scope=%d\n",
                               ifa->ifa_index, ifa->ifa_family, ifa->ifa_prefixlen,
                               addr, ifa->ifa_scope);
                    }
                }
            } else if (type == RTM_GETROUTE) {
                struct rtmsg *rtm = NLMSG_DATA(nh);
                struct rtattr *rta = RTM_RTA(rtm);
                int rtl = RTM_PAYLOAD(nh);
                char dst[INET6_ADDRSTRLEN] = "", gate[INET6_ADDRSTRLEN] = "", src[INET6_ADDRSTRLEN] = "";
                int oif = 0;
                for (; RTA_OK(rta, rtl); rta = RTA_NEXT(rta, rtl)) {
                    if (rta->rta_type == RTA_DST) {
                        if (rtm->rtm_family == AF_INET)
                            inet_ntop(AF_INET, RTA_DATA(rta), dst, sizeof(dst));
                        else if (rtm->rtm_family == AF_INET6)
                            inet_ntop(AF_INET6, RTA_DATA(rta), dst, sizeof(dst));
                    } else if (rta->rta_type == RTA_GATEWAY) {
                        if (rtm->rtm_family == AF_INET)
                            inet_ntop(AF_INET, RTA_DATA(rta), gate, sizeof(gate));
                        else if (rtm->rtm_family == AF_INET6)
                            inet_ntop(AF_INET6, RTA_DATA(rta), gate, sizeof(gate));
                    } else if (rta->rta_type == RTA_PREFSRC) {
                        if (rtm->rtm_family == AF_INET)
                            inet_ntop(AF_INET, RTA_DATA(rta), src, sizeof(src));
                        else if (rtm->rtm_family == AF_INET6)
                            inet_ntop(AF_INET6, RTA_DATA(rta), src, sizeof(src));
                    } else if (rta->rta_type == RTA_OIF) {
                        oif = *(int*)RTA_DATA(rta);
                    }
                }
                printf("ROUTE: family=%d table=%d dst=%s gateway=%s src=%s oif=%d\n",
                       rtm->rtm_family, rtm->rtm_table, dst, gate, src, oif);
            }
        }
    }
    close(fd);
}

/* Read /proc/net/tcp, udp, raw, unix for listeners */
static void dump_proc_net(const char *file, const char *proto) {
    FILE *f = fopen(file, "r");
    if (!f) return;
    char line[256];
    if (!fgets(line, sizeof(line), f)) { fclose(f); return; } /* skip header */
    while (fgets(line, sizeof(line), f)) {
        int sl, local_port, rem_port, state, uid, inode;
        char local_addr[64], rem_addr[64];
        /* Format: sl local_address rem_address st tx_queue rx_queue tr tm->when retrnsmt uid timeout inode */
        if (sscanf(line, "%d: %64[0-9A-Fa-f:]:%X %64[0-9A-Fa-f:]:%X %X %*x %*x %*x %*x %*x %d %*d %d",
                   &sl, local_addr, &local_port, rem_addr, &rem_port, &state, &uid, &inode) >= 6) {
            printf("LISTENER: proto=%s local=%s:%d state=%d uid=%d inode=%d\n",
                   proto, local_addr, local_port, state, uid, inode);
        }
    }
    fclose(f);
}

/* Read /etc/resolv.conf for DNS servers */
static void dump_resolv(void) {
    FILE *f = fopen("/etc/resolv.conf", "r");
    if (!f) return;
    char line[256];
    while (fgets(line, sizeof(line), f)) {
        if (strncmp(line, "nameserver", 10) == 0) {
            char ns[64];
            if (sscanf(line, "nameserver %63s", ns) == 1) {
                printf("DNS: nameserver=%s\n", ns);
            }
        }
    }
    fclose(f);
}

int main(void) {
    dump_netlink(RTM_GETLINK);
    dump_netlink(RTM_GETADDR);
    dump_netlink(RTM_GETROUTE);

    dump_proc_net("/proc/net/tcp", "tcp");
    dump_proc_net("/proc/net/tcp6", "tcp6");
    dump_proc_net("/proc/net/udp", "udp");
    dump_proc_net("/proc/net/udp6", "udp6");
    dump_proc_net("/proc/net/raw", "raw");
    dump_proc_net("/proc/net/raw6", "raw6");
    dump_proc_net("/proc/net/unix", "unix");

    dump_resolv();
    return 0;
}
