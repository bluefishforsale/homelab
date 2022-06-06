set -x
echo "Start to scanning Orphaned Pod. Orphaned directory will be umounted if it is mounted, and will be removed if it is empty."

source ${K8S_HOME}/bin/env.sh

IFS=$'\r\n'
for ((i=1; i<=5; i++));
do
    orphanExist="false"
    for podid in `grep "errors similar to this. Turn up verbosity to see them." /var/log/messages | tail -1 | awk '{print $12}' | sed 's/"//g'`;
    do
        echo $podid

        # not process if the volume directory is not exist.
        if [ ! -d ${KUBELET_HOME}/kubelet/pods/$podid/volumes/ ]; then
            continue
        fi

        # umount subpath if exist
        if [ -d ${KUBELET_HOME}/kubelet/pods/$podid/volume-subpaths/ ]; then
            mountpath=`mount | grep ${KUBELET_HOME}/kubelet/pods/$podid/volume-subpaths/ | awk '{print $3}'`
            for mntPath in $mountpath;
            do
                echo "umount subpath $mntPath"
                umount $mntPath
            done
        fi

        orphanExist="true"
        volumeTypes=`ls ${KUBELET_HOME}/kubelet/pods/$podid/volumes/`
        for volumeType in $volumeTypes;
        do
            subVolumes=`ls -A ${KUBELET_HOME}/kubelet/pods/$podid/volumes/$volumeType`
            if [ "$subVolumes" != "" ]; then
                echo "${KUBELET_HOME}/kubelet/pods/$podid/volumes/$volumeType contents volume: $subVolumes"
                for subVolume in $subVolumes;
                do
                    # check subvolume path is mounted or not
                    findmnt ${KUBELET_HOME}/kubelet/pods/$podid/volumes/$volumeType/$subVolume
                    if [ "$?" != "0" ]; then
                        echo "${KUBELET_HOME}/kubelet/pods/$podid/volumes/$volumeType/$subVolume is not mounted, just need to remove"
                        rm -rf ${KUBELET_HOME}/kubelet/pods/$podid/volumes/$volumeType/$subVolume
                    else
                        echo "${KUBELET_HOME}/kubelet/pods/$podid/volumes/$volumeType/$subVolume is mounted, umount it"
                        umount ${KUBELET_HOME}/kubelet/pods/$podid/volumes/$volumeType/$subVolume
                        rm -rf ${KUBELET_HOME}/kubelet/pods/$podid/volumes/$volumeType/$subVolume
                    fi
                done
            else
                rm -rf ${KUBELET_HOME}/kubelet/pods/$podid
            fi
        done
    done
    if [ "$orphanExist" = "false" ]; then
        break
    fi
    sleep 2
done