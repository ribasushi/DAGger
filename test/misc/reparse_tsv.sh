grep -v unreachable convergence_rawdata.tsv \
| perl -MData::Dumper -ne '
  chomp;
  my %f = split /[\t\:]/, $_;
  $statz->{join ",", @f{qw(Data CidVer Chunker Trickle RawLeaves Inlining)}}->{$f{CID}}->{$f{Impl}} = 1;
}{
  print join ",", qw( Input CidVer Chunker TrickleDag UseRawLeaves UseInlining GoCID JsCID );
  print "\n";
    for my $k ( sort keys %$statz ) {
      for my $c ( sort keys %{ $statz->{$k} } ) {
        printf( "%s,%s,%s,\n", $k,
          ($statz->{$k}{$c}{go} ? $c : "N/A" ),
          ($statz->{$k}{$c}{js} ? $c : "N/A"),
        );
      }
    }
' > convergence_mismatches.csv
