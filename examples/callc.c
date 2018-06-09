 __attribute__((cdecl)) long long phase(long long a, long long b, char toggle) {
	long long result = a * b;
	if (toggle) {
		return result + 2018;	
	} else {
		return result;
	}
}