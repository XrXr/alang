struct a {
	_Bool hoho;
	int faker;
	_Bool wrench;
	long long jojo;
	_Bool another;
	int fun;
};

void fillStruct(struct a *out) {
	out->hoho = 1;
	out->faker = sizeof (struct a);
	out->wrench = 0;
	out->jojo = ((char*)&out->hoho - (char*)out) + ((char*)&out->faker - (char*)out) + ((char*)&out->wrench - (char*)out) + ((char*)&out->jojo - (char*)out) + ((char*)&out->another - (char*)out);
	out->another = 1;
	out->fun = 8388193;
}
