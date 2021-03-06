#! /usr/bin/env python
import unittest
import subprocess

class DockerCephTests(unittest.TestCase):
    IMAGE="docker-test-volume"

    def tearDown(self):
        run("rbd rm " + self.IMAGE)

    def test_docker_ceph_volume(self):
        out = run("docker run -t -v {}:/bar:ceph debian:latest ls -l /bar".format(self.IMAGE))
        self.assertIn("lost+found", out)
        out = run("docker volume ls")
        self.assertIn(self.IMAGE, out)
        out = run("rbd showmapped")
        self.assertNotIn(self.IMAGE, out)

    def test_docker_ceph_initialize_fs(self):
        out = run("rbd create --size=1G {}".format(self.IMAGE))
        out = run("docker run -t -v {}:/bar:ceph debian:latest ls -l /bar".format(self.IMAGE))
        self.assertIn("lost+found", out)

    def test_docker_ceph_dont_initialize_fs(self):
        out = run("rbd create --size=1G {}".format(self.IMAGE))
        dev = run("rbd map {} 2>/dev/null".format(self.IMAGE)).strip()
        out = run("mkfs.ext4 -m0 {}".format(dev))
        tmpdir = run("mktemp -d ").strip()
        out = run("mount {} {}".format(dev, tmpdir))
        out = run("touch {}/testfile".format(tmpdir))
        out = run("umount {}".format(dev))
        out = run("rbd unmap {}".format(self.IMAGE))
        out = run("docker run -t -v {}:/bar:ceph debian:latest ls -l /bar".format(self.IMAGE))
        self.assertIn("testfile", out)

    def test_docker_ceph_auto_resize(self):
        show_fs_size = "docker run -t -v {}:/bar:ceph debian:latest df -h --output=size /bar".format(self.IMAGE)
        out = run("rbd create --size=1G {}".format(self.IMAGE))
        out = run(show_fs_size)
        self.assertIn("976M", out)
        out = run("rbd resize --size=2G {}".format(self.IMAGE))
        out = run(show_fs_size)
        self.assertIn("2.0G", out)

    def test_docker_ceph_luks_volume(self):
        # create encripted luks volume
        out = run("rbd create --size=1G {}".format(self.IMAGE))
        dev = run("rbd map {} 2>/dev/null".format(self.IMAGE)).strip()
        out = run("echo '{}' | cryptsetup luksFormat -q {}".format(self.IMAGE, dev))
        out = run("rbd unmap {}".format(self.IMAGE))
        create_file = "docker run -t -v {}:/foo:ceph debian:latest /bin/bash -c \"echo 'dog' > /foo/cat\"".format(self.IMAGE)
        out = run(create_file)
        read_file = "docker run -t -v {}:/foo:ceph debian:latest cat /foo/cat".format(self.IMAGE)
        out = run(read_file)
        self.assertIn("dog", out)
        out = run("rbd showmapped")
        self.assertNotIn("docker-test-volume", out)

    def test_docker_ceph_luks_with_wrong_key(self):
        # create encripted luks volume
        out = run("rbd create --size=1G {}".format(self.IMAGE))
        dev = run("rbd map {} 2>/dev/null".format(self.IMAGE)).strip()
        out = run("echo '{}' | cryptsetup luksFormat -q {}".format('wrong_key', dev))
        out = run("rbd unmap {}".format(self.IMAGE))
        # docker expects the volume to be encrypted using its name as the key.
        self.assertRaises(subprocess.CalledProcessError, run, "docker run -t -v {}:/foo:ceph debian:latest echo hello".format(self.IMAGE))

class DockerCephPoolsTests(DockerCephTests):
    IMAGE="testpool/docker-test-volume"


def run(cmd):
    """
    Executes the given command returning a tuple with the return value and the stdout and stderr.
    :param cmd: the command to run
    :return: a tuple containing the return code and the output of the command execution
    """
    use_shell = isinstance(cmd, basestring)
    out = subprocess.check_output(cmd, stderr=subprocess.STDOUT, shell=use_shell, universal_newlines=True)
    return out


if __name__ == '__main__':
    unittest.main()
