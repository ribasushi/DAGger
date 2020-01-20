#!/bin/bash

go_adder="$HOME/devel/go-ipfs/cmd/ipfs/ipfs"
js_adder="jsipfs"

for data_fn in ../data/*.zst ; do \
	for use_inlining in false true ; do \
		for use_trickle in false true ; do \
			for use_rawleaves in false true ; do \
				for cidver in 0 1 ; do \
					for use_chunker in \
						size-15997 \
						size-65535 \
						size-262144 \
						size-1048576 \
						buzhash \
						rabin \
						rabin-262144-524288-1048576 \
						rabin-65535-262144-524288 \
						rabin-256-65536-262144 \
					; do \
						for use_implementation in go js ; do \
							[[ "$use_implementation" == "js" ]] && ( [[ "$use_inlining" == "true" ]] || [[ "$use_chunker" == "buzhash" ]] || [[ "$cidver" == "0" ]] ) && continue


							adder_varname="${use_implementation}_adder"

							core_opts=()
							add_opts=()
							
							if [[ "$cidver" == "0" ]] ; then
								core_opts+=( "--upgrade-cidv0-in-output=true" )
								add_opts+=( "--cid-version=0" )
							else
								add_opts+=( "--cid-version=1" )
							fi
							
							if [ "$use_implementation" == "go" ] ; then
								add_opts+=( "--inline=$use_inlining" )
							fi

							echo -en "Data:$data_fn\tImpl:$use_implementation\tTrickle:$use_trickle\tRawLeaves:$use_rawleaves\tInlining:$use_inlining\tCidVer:$cidver\tChunker:$use_chunker\t"
							echo -en "Cmd:${core_opts[@]} add --chunker=$use_chunker --trickle=$use_trickle --raw-leaves=$use_rawleaves ${add_opts[@]}\tCID:"

							zstd -qdck $data_fn | ${!adder_varname} ${core_opts[@]} add -n --quiet --chunker=$use_chunker --trickle=$use_trickle --raw-leaves=$use_rawleaves ${add_opts[@]}
						done
					done
				done
			done
		done
	done
done
