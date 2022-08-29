import os

def alter_context(context):
    """ Modify the context and return it """
    context['tag'] = os.environ['GITHUB_REF_NAME']
    return context

def extra_filters():
    """ Declare some custom filters.
        Returns: dict(name = function)
    """
    return dict(
        sha256=lambda f: getChecksum(f),
    )

f = open("dist/checksums.txt", "r")
sums = f.readlines()
f.close()

def getChecksum(filename):
    for line in sums:
        if line.find(filename.strip()) >= 0:
            return line.split(' ')[0]
