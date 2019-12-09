#!/bin/bash
ROOT_UID=0
SUCCESS=0
E_USEREXISTS=70

# Run as root, of course. (this might not be necessary, because we have to run the script somehow with root anyway)
if [ "$UID" -ne "$ROOT_UID" ]
then
  echo "Must be root to run this script."
  exit $E_NOTROOT
fi  

#test, if both argument are there
if [ $# -eq 1 ]; then
username=$1

	# Check if user already exists.
	grep -q "$username" /etc/passwd
	if [ $? -eq $SUCCESS ] 
	then	
	echo "User $username does already exist."
  	echo "please chose another username."
	exit $E_USEREXISTS
	fi  

	useradd -m -d /home/$username -s /bin/bash $username
	mkdir /home/$username/.ssh/
	touch /home/$username/.ssh/authorized_keys
	cp /home/davide/tmp/id_rsa.pub /home/$username/.ssh/authorized_keys #change davide,tmp,key
	chown -R $username:$username /home/$username/.ssh
        chmod 700 /home/$username/.ssh
	chmod 600 /home/$username/.ssh/authorized_keys

	echo "the account is setup"

else
        echo  " this program needs 1 arguments you have given $# "
        echo  " you have to call the script $0 username and the pass "
fi

exit 0
