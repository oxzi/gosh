function goshy 
	for option in $argv
		switch "$option"
        		case -b --burn
				set burn "1"
		        case \*
				set file "@"$option
		end
	end
	curl -F "file=$file" -F "burn=$burn" http://our-server.example/
end
