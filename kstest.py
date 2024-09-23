import scipy
import sys


# print("p-value: ", prob)

def main():
    D = sys.argv[1]
    N = sys.argv[2]

    prob = scipy.stats.kstwo.sf(float(D), int(N))
    print("p-value: prob")